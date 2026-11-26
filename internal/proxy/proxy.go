// Package proxy 实现请求信封 → http.Request 的构建与转发，契约见 docs/api.md。
package proxy

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// 尺寸与数量限制（docs/api.md §1）。
const (
	MaxCaptureBytes   = 20 << 20 // 响应捕获上限 20MB
	MaxBodyTextBytes  = 2 << 20  // body.text ≤ 2MB
	MaxFileRawBytes   = 14 << 20 // 单文件解码后 ≤ 14MB
	MaxHeaders        = 128
	MaxFields         = 64
	MaxHeaderValueLen = 64 << 10
	MaxTimeoutMs      = 300_000
	InternalTimeout   = 300 * time.Second // timeoutMs=0（不限）时的服务端硬顶
)

type Header struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type BodyField struct {
	Name          string `json:"name"`
	Type          string `json:"type"` // text | file（仅 form-data）
	Value         string `json:"value"`
	Enabled       bool   `json:"enabled"`
	FileName      string `json:"fileName"`
	ContentType   string `json:"contentType"`
	ContentBase64 string `json:"contentBase64"`
}

type Body struct {
	Type   string      `json:"type"` // none|json|x-www-form-urlencoded|form-data|raw|binary
	Text   string      `json:"text,omitempty"`
	Mime   string      `json:"mime,omitempty"` // 仅 raw
	Fields []BodyField `json:"fields,omitempty"`
}

type Options struct {
	TimeoutMs          int  `json:"timeoutMs"`
	FollowRedirects    bool `json:"followRedirects"`
	InsecureSkipVerify bool `json:"insecureSkipVerify"`
}

// RequestEnvelope POST /api/proxy 的请求体。Record 由 handler 层消费，proxy 不关心。
type RequestEnvelope struct {
	Method  string   `json:"method"`
	URL     string   `json:"url"`
	Headers []Header `json:"headers"`
	Body    Body     `json:"body"`
	Options Options  `json:"options"`
	Record  *bool    `json:"record"` // nil = true
}

func (e *RequestEnvelope) ShouldRecord() bool { return e.Record == nil || *e.Record }

type ResponseHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Response struct {
	Status     int              `json:"status"`
	StatusText string           `json:"statusText"`
	Headers    []ResponseHeader `json:"headers"`
	BodyText   string           `json:"bodyText,omitempty"` // UTF-8 可解码时给出
	BodyBase64 string           `json:"bodyBase64"`
	IsText     bool             `json:"isText"`
	Truncated  bool             `json:"truncated"`
	SizeBytes  int64            `json:"sizeBytes"`
	TimeMs     int64            `json:"timeMs"`
}

// ExecError 结构化业务错误（docs/api.md §0），HTTPStatus 为应答状态码。
type ExecError struct {
	HTTPStatus int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *ExecError) Error() string { return e.Code + ": " + e.Message }

func errInvalid(format string, a ...any) *ExecError {
	return &ExecError{HTTPStatus: http.StatusBadRequest, Code: "invalid_request", Message: fmt.Sprintf(format, a...)}
}

func errUpstream(format string, a ...any) *ExecError {
	return &ExecError{HTTPStatus: http.StatusBadGateway, Code: "upstream_unreachable", Message: fmt.Sprintf(format, a...)}
}

func errTimeout(format string, a ...any) *ExecError {
	return &ExecError{HTTPStatus: http.StatusGatewayTimeout, Code: "upstream_timeout", Message: fmt.Sprintf(format, a...)}
}

// Execute 构建并转发请求。上游 4xx/5xx 属成功路径（状态透传）；
// 传输失败 502、超时 504、信封非法 400。
func Execute(env RequestEnvelope) (Response, *ExecError) {
	if err := Validate(&env); err != nil {
		return Response{}, err
	}

	req, rerr := buildRequest(&env)
	if rerr != nil {
		return Response{}, rerr
	}

	client := &http.Client{Timeout: InternalTimeout}
	if !env.Options.FollowRedirects {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // 停在首个 3xx，响应透传
		}
	}
	if env.Options.InsecureSkipVerify {
		tr := http.DefaultTransport.(*http.Transport).Clone()      //nolint:forcetypeassert // DefaultTransport 固定为 *http.Transport
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // 用户显式要求跳过校验（-k）
		client.Transport = tr
	}
	if env.Options.TimeoutMs > 0 {
		client.Timeout = time.Duration(env.Options.TimeoutMs) * time.Millisecond
	}

	start := time.Now()
	resp, derr := client.Do(req)
	if derr != nil {
		var netErr net.Error
		if errors.As(derr, &netErr) && netErr.Timeout() {
			return Response{}, errTimeout("上游超时: %v", derr)
		}
		return Response{}, errUpstream("%v", derr)
	}
	defer resp.Body.Close()

	captured, truncated, bodyErr := readUpTo(resp.Body, MaxCaptureBytes)
	if bodyErr != nil {
		return Response{}, errUpstream("读取上游响应中断: %v", bodyErr)
	}
	elapsed := time.Since(start).Milliseconds()

	size := int64(len(captured))
	if resp.ContentLength >= 0 {
		size = resp.ContentLength
	}

	out := Response{
		Status:     resp.StatusCode,
		StatusText: statusText(resp.Status),
		Headers:    flattenHeaders(resp.Header),
		BodyBase64: base64.StdEncoding.EncodeToString(captured),
		IsText:     utf8.Valid(captured),
		Truncated:  truncated,
		SizeBytes:  size,
		TimeMs:     elapsed,
	}
	if out.IsText {
		out.BodyText = string(captured)
	}
	return out, nil
}

// Validate 校验信封合法性与尺寸限制（docs/api.md §1）。
func Validate(env *RequestEnvelope) *ExecError {
	env.Method = strings.TrimSpace(env.Method)
	if env.Method == "" || !isToken(env.Method) {
		return errInvalid("method 非法: %q", env.Method)
	}
	u, err := url.Parse(env.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errInvalid("url 非法: %q", env.URL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errInvalid("仅支持 http/https，收到 %q", u.Scheme)
	}
	if len(env.Headers) > MaxHeaders {
		return errInvalid("headers 超过 %d 条", MaxHeaders)
	}
	for _, h := range env.Headers {
		if h.Name == "" { // 空名行发送时忽略（api.md §1）
			continue
		}
		if !isToken(h.Name) {
			return errInvalid("header 名非法: %q", h.Name)
		}
		if len(h.Value) > MaxHeaderValueLen {
			return errInvalid("header %s 的值超过 64KB", h.Name)
		}
		if hasCTL(h.Name) || hasCTL(h.Value) {
			return errInvalid("header %s 含控制字符", h.Name)
		}
	}
	if len(env.Body.Fields) > MaxFields {
		return errInvalid("fields 超过 %d 条", MaxFields)
	}
	if len(env.Body.Text) > MaxBodyTextBytes {
		return errInvalid("body.text 超过 2MB")
	}
	if env.Options.TimeoutMs < 0 || env.Options.TimeoutMs > MaxTimeoutMs {
		return errInvalid("timeoutMs 须在 0~%d 内", MaxTimeoutMs)
	}
	switch env.Body.Type {
	case "", "none", "json", "raw", "binary":
	case "x-www-form-urlencoded":
		for _, f := range env.Body.Fields {
			if f.Type == "file" {
				return errInvalid("x-www-form-urlencoded 不支持文件字段 %q", f.Name)
			}
		}
	case "form-data":
		for _, f := range env.Body.Fields {
			if f.ContentBase64 == "" {
				continue
			}
			raw, derr := base64.StdEncoding.DecodeString(f.ContentBase64)
			if derr != nil {
				return errInvalid("字段 %s 的 contentBase64 不是合法 base64", f.Name)
			}
			if len(raw) > MaxFileRawBytes {
				return errInvalid("字段 %s 的文件内容超过 14MB", f.Name)
			}
		}
	default:
		return errInvalid("body.type 非法: %q", env.Body.Type)
	}
	if env.Body.Type == "binary" && env.Body.Text != "" {
		if _, derr := base64.StdEncoding.DecodeString(env.Body.Text); derr != nil {
			return errInvalid("binary 的 text 须为合法 base64")
		}
	}
	return nil
}

// buildRequest 把信封构建为 http.Request（代理构建规则见 api.md §1 末节）。
func buildRequest(env *RequestEnvelope) (*http.Request, *ExecError) {
	var body []byte
	var autoCT string

	switch env.Body.Type {
	case "", "none": // none 为历史数据兼容（v0.2.4 起前端不再产生）
	case "json":
		if env.Body.Text != "" { // 空文本 = 无 body、不加 CT（api.md 代理构建规则）
			body = []byte(env.Body.Text)
			autoCT = "application/json"
		}
	case "raw":
		if env.Body.Text != "" {
			body = []byte(env.Body.Text)
			autoCT = strings.TrimSpace(env.Body.Mime)
			if autoCT == "" {
				autoCT = "text/plain"
			}
		}
	case "binary":
		// Validate 已确保 text 为合法 base64；空串 = 空文件体 → 不发送。
		if env.Body.Text != "" {
			body, _ = base64.StdEncoding.DecodeString(env.Body.Text)
			autoCT = "application/octet-stream"
		}
	case "x-www-form-urlencoded":
		vals := url.Values{}
		for _, f := range env.Body.Fields {
			if !f.Enabled || f.Name == "" {
				continue
			}
			vals.Set(f.Name, f.Value)
		}
		body = []byte(vals.Encode())
		autoCT = "application/x-www-form-urlencoded"
	case "form-data":
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, f := range env.Body.Fields {
			if !f.Enabled || f.Name == "" {
				continue
			}
			if f.Type == "file" {
				content, derr := base64.StdEncoding.DecodeString(f.ContentBase64)
				if derr != nil {
					return nil, errInvalid("字段 %s 的 contentBase64 不是合法 base64", f.Name)
				}
				fileName := f.FileName
				if fileName == "" {
					fileName = f.Name
				}
				ct := f.ContentType
				if ct == "" {
					ct = "application/octet-stream"
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition",
					fmt.Sprintf(`form-data; name=%q; filename=%q`, f.Name, fileName))
				h.Set("Content-Type", ct)
				part, perr := mw.CreatePart(h)
				if perr != nil {
					return nil, errUpstream("构建 multipart 失败: %v", perr)
				}
				if _, perr = part.Write(content); perr != nil {
					return nil, errUpstream("构建 multipart 失败: %v", perr)
				}
			} else if werr := mw.WriteField(f.Name, f.Value); werr != nil {
				return nil, errUpstream("构建 multipart 失败: %v", werr)
			}
		}
		if cerr := mw.Close(); cerr != nil {
			return nil, errUpstream("构建 multipart 失败: %v", cerr)
		}
		body = buf.Bytes()
		autoCT = mw.FormDataContentType()
	default:
		return nil, errInvalid("body.type 非法: %q", env.Body.Type)
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(env.Method, env.URL, reader)
	if err != nil {
		return nil, errInvalid("构建请求失败: %v", err)
	}

	userHasCT := false
	for _, h := range env.Headers {
		if !h.Enabled || h.Name == "" {
			continue
		}
		if strings.EqualFold(h.Name, "Host") {
			req.Host = h.Value
			continue
		}
		if strings.EqualFold(h.Name, "Content-Length") {
			continue // 由 body 长度推导，忽略用户值
		}
		if strings.EqualFold(h.Name, "Content-Type") {
			userHasCT = true
		}
		req.Header.Add(h.Name, h.Value)
	}
	if autoCT != "" && !userHasCT {
		req.Header.Set("Content-Type", autoCT)
	}
	return req, nil
}

// readUpTo 读取至多 limit 字节；读到 limit+1 说明超限，截断并标记。
func readUpTo(r io.Reader, limit int64) (data []byte, truncated bool, err error) {
	all, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(all)) > limit {
		return all[:limit], true, nil
	}
	return all, false, nil
}

func flattenHeaders(h http.Header) []ResponseHeader {
	names := make([]string, 0, len(h))
	for k := range h {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]ResponseHeader, 0, len(names))
	for _, k := range names {
		for _, v := range h[k] {
			out = append(out, ResponseHeader{Name: k, Value: v})
		}
	}
	return out
}

// statusText 从 "200 OK" 提取 "OK"；非标准短语返回空串。
func statusText(status string) string {
	if i := strings.IndexByte(status, ' '); i >= 0 {
		return strings.TrimSpace(status[i+1:])
	}
	return ""
}

// isToken RFC 7230 token 字符。
func isToken(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isTokenChar(c) {
			return false
		}
	}
	return len(s) > 0
}

func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// hasCTL 拒绝 header 名/值中的 CR/LF/NUL 等（防注入，见测试 P18）。
func hasCTL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}
