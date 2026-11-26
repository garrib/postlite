package proxy

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// upcap 捕获上游收到的方法/头/体/Host。
type upcap struct {
	mu     sync.Mutex
	method string
	header http.Header
	body   []byte
	host   string
}

func (c *upcap) record(r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.method = r.Method
	c.header = r.Header.Clone()
	c.body = b
	c.host = r.Host
}

func (c *upcap) get() (string, http.Header, []byte, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.method, c.header, c.body, c.host
}

// echoUpstream 记录请求并回 200 "ok"。
func echoUpstream(c *upcap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.record(r)
		_, _ = w.Write([]byte("ok"))
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func hdr(name, value string) Header {
	return Header{Name: name, Value: value, Enabled: true}
}

// TestExecute 转发保真表驱动（docs/test-cases.md §3.1 P1-P20）。
func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  func(baseURL string) RequestEnvelope
		// upstream 为 nil 时使用 echoUpstream；redirect/大响应等特例自行给 handler。
		upstream func() (*httptest.Server, *upcap)
		check    func(t *testing.T, resp Response, perr *ExecError, cap *upcap)
	}{
		{
			name: "P1 GET 无 body，无自动 CT",
			env:  func(u string) RequestEnvelope { return RequestEnvelope{Method: "GET", URL: u} },
			check: func(t *testing.T, resp Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				m, h, body, _ := cap.get()
				if m != "GET" || len(body) != 0 {
					t.Fatalf("上游收到 %s body=%q", m, body)
				}
				if ct := h.Get("Content-Type"); ct != "" {
					t.Fatalf("不应有 Content-Type，得到 %q", ct)
				}
				if resp.Status != 200 || resp.BodyText != "ok" {
					t.Fatalf("resp=%+v", resp)
				}
			},
		},
		{
			name: "P2a json 未显式 CT → 自动 application/json",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "json", Text: `{"a":1}`}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if got := h.Get("Content-Type"); got != "application/json" {
					t.Fatalf("CT=%q", got)
				}
				if string(body) != `{"a":1}` {
					t.Fatalf("body=%q", body)
				}
			},
		},
		{
			name: "P2b json 显式 CT 不被覆盖",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method:  "POST",
					URL:     u,
					Headers: []Header{hdr("Content-Type", "application/json; charset=utf-8")},
					Body:    Body{Type: "json", Text: `{}`},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, _, _ := cap.get()
				if got := h.Get("Content-Type"); got != "application/json; charset=utf-8" {
					t.Fatalf("CT=%q", got)
				}
			},
		},
		{
			name: "P3 urlencoded：enabled 过滤 + urlencode + 自动 CT",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "POST", URL: u,
					Body: Body{Type: "x-www-form-urlencoded", Fields: []BodyField{
						{Name: "a", Value: "1", Enabled: true, Type: "text"},
						{Name: "b", Value: "中&=文", Enabled: true, Type: "text"},
						{Name: "c", Value: "3", Enabled: false, Type: "text"},
					}},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if got := h.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
					t.Fatalf("CT=%q", got)
				}
				if string(body) != "a=1&b=%E4%B8%AD%26%3D%E6%96%87" {
					t.Fatalf("body=%q", body)
				}
			},
		},
		{
			name: "P4 form-data 文本+文件字段",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "POST", URL: u,
					Body: Body{Type: "form-data", Fields: []BodyField{
						{Name: "k", Value: "v", Enabled: true, Type: "text"},
						{Name: "f", Enabled: true, Type: "file", FileName: "a.txt", ContentType: "text/plain", ContentBase64: b64("hello")},
					}},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if !strings.HasPrefix(h.Get("Content-Type"), "multipart/form-data; boundary=") {
					t.Fatalf("CT=%q", h.Get("Content-Type"))
				}
				r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
				r.Header = h
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatalf("ParseMultipartForm: %v", err)
				}
				if got := r.FormValue("k"); got != "v" {
					t.Fatalf("k=%q", got)
				}
				fh, err := r.MultipartForm.File["f"][0].Open()
				if err != nil {
					t.Fatalf("打开文件字段失败: %v", err)
				}
				content, _ := io.ReadAll(fh)
				if string(content) != "hello" {
					t.Fatalf("file=%q", content)
				}
				if r.MultipartForm.File["f"][0].Filename != "a.txt" {
					t.Fatalf("filename=%q", r.MultipartForm.File["f"][0].Filename)
				}
			},
		},
		{
			name: "P5 form-data 文件无内容 → 空文件体",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "POST", URL: u,
					Body: Body{Type: "form-data", Fields: []BodyField{
						{Name: "f", Enabled: true, Type: "file", FileName: "x.bin"},
					}},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
				r.Header = h
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Fatalf("ParseMultipartForm: %v", err)
				}
				fh, err := r.MultipartForm.File["f"][0].Open()
				if err != nil {
					t.Fatalf("打开失败: %v", err)
				}
				content, _ := io.ReadAll(fh)
				if len(content) != 0 {
					t.Fatalf("应为空，得到 %q", content)
				}
			},
		},
		{
			name: "P6a raw 带 mime",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "raw", Text: "<a/>", Mime: "application/xml"}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if h.Get("Content-Type") != "application/xml" || string(body) != "<a/>" {
					t.Fatalf("CT=%q body=%q", h.Get("Content-Type"), body)
				}
			},
		},
		{
			name: "P6b raw mime 缺省 text/plain",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "raw", Text: "hi"}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, _, _ := cap.get()
				if h.Get("Content-Type") != "text/plain" {
					t.Fatalf("CT=%q", h.Get("Content-Type"))
				}
			},
		},
		{
			name: "P7 binary 字节保真",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "binary", Text: base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0xff})}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if !bytes.Equal(body, []byte{0x00, 0x01, 0xff}) {
					t.Fatalf("body=%x", body)
				}
				if h.Get("Content-Type") != "application/octet-stream" {
					t.Fatalf("CT=%q", h.Get("Content-Type"))
				}
			},
		},
		{
			name: "P8 Host header → req.Host",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "GET", URL: u,
					Headers: []Header{hdr("Host", "myhost.example")},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, _, _, host := cap.get()
				if host != "myhost.example" {
					t.Fatalf("Host=%q", host)
				}
			},
		},
		{
			name: "P9 用户 Content-Length 被忽略",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "POST", URL: u,
					Headers: []Header{hdr("Content-Length", "999")},
					Body:    Body{Type: "raw", Text: "abc"},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if string(body) != "abc" {
					t.Fatalf("body=%q", body)
				}
				// CL 由 body 长度推导（3），不是用户的 999。
				if h.Get("Content-Length") != "3" {
					t.Fatalf("上游 CL=%q", h.Get("Content-Length"))
				}
			},
		},
		{
			name: "P10 重复同名 header",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "GET", URL: u,
					Headers: []Header{hdr("X-Multi", "a"), hdr("X-Multi", "b")},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, _, _ := cap.get()
				if got := len(h.Values("X-Multi")); got != 2 {
					t.Fatalf("X-Multi 应有 2 条，得到 %d", got)
				}
			},
		},
		{
			name: "P11 非 http(s) scheme → 400",
			env:  func(_ string) RequestEnvelope { return RequestEnvelope{Method: "GET", URL: "ftp://x/a"} },
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 400 || perr.Code != "invalid_request" {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P12 上游 404 透传为成功信封",
			upstream: func() (*httptest.Server, *upcap) {
				c := &upcap{}
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "nope", http.StatusNotFound)
				}))
				return s, c
			},
			env: func(u string) RequestEnvelope { return RequestEnvelope{Method: "GET", URL: u} },
			check: func(t *testing.T, resp Response, perr *ExecError, _ *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				if resp.Status != 404 || resp.StatusText != "Not Found" {
					t.Fatalf("resp=%+v", resp)
				}
			},
		},
		{
			name: "P13 默认不跟随 3xx",
			upstream: func() (*httptest.Server, *upcap) {
				c := &upcap{}
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/a" {
						http.Redirect(w, r, "/b", http.StatusFound)
						return
					}
					_, _ = w.Write([]byte("done"))
				}))
				return s, c
			},
			env: func(u string) RequestEnvelope { return RequestEnvelope{Method: "GET", URL: u + "/a"} },
			check: func(t *testing.T, resp Response, perr *ExecError, _ *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				if resp.Status != 302 {
					t.Fatalf("status=%d", resp.Status)
				}
				found := false
				for _, h := range resp.Headers {
					if h.Name == "Location" {
						found = true
					}
				}
				if !found {
					t.Fatalf("应能看见 Location 头")
				}
			},
		},
		{
			name: "P14 followRedirects → 最终 200",
			upstream: func() (*httptest.Server, *upcap) {
				c := &upcap{}
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/a" {
						http.Redirect(w, r, "/b", http.StatusFound)
						return
					}
					_, _ = w.Write([]byte("done"))
				}))
				return s, c
			},
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "GET", URL: u + "/a", Options: Options{FollowRedirects: true}}
			},
			check: func(t *testing.T, resp Response, perr *ExecError, _ *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				if resp.Status != 200 || resp.BodyText != "done" {
					t.Fatalf("resp=%+v", resp)
				}
			},
		},
		{
			name: "P16 超时 → 504",
			upstream: func() (*httptest.Server, *upcap) {
				c := &upcap{}
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					time.Sleep(300 * time.Millisecond) // 客户端 50ms 超时后即断开
				}))
				return s, c
			},
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "GET", URL: u, Options: Options{TimeoutMs: 50}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 504 || perr.Code != "upstream_timeout" {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P17 超 20MB 截断",
			upstream: func() (*httptest.Server, *upcap) {
				c := &upcap{}
				payload := bytes.Repeat([]byte("a"), 21<<20)
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					// 显式 Content-Length，保证 sizeBytes 取到完整 21MB 而非 chunked 的 -1。
					w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
					_, _ = w.Write(payload)
				}))
				return s, c
			},
			env: func(u string) RequestEnvelope { return RequestEnvelope{Method: "GET", URL: u} },
			check: func(t *testing.T, resp Response, perr *ExecError, _ *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				if !resp.Truncated {
					t.Fatalf("应 truncated")
				}
				if resp.SizeBytes != 21<<20 {
					t.Fatalf("sizeBytes=%d 期望 %d", resp.SizeBytes, 21<<20)
				}
				dec, err := base64.StdEncoding.DecodeString(resp.BodyBase64)
				if err != nil || len(dec) != MaxCaptureBytes {
					t.Fatalf("捕获 %d 字节", len(dec))
				}
			},
		},
		{
			name: "P18 header 值含 CRLF → 400",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{
					Method: "GET", URL: u,
					Headers: []Header{hdr("X-Evil", "a\r\nSet-Cookie: pwned=1")},
				}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 400 {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P19a body.text 超 2MB → 400",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "raw", Text: strings.Repeat("a", MaxBodyTextBytes+1)}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 400 {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P19b headers 超 128 → 400",
			env: func(u string) RequestEnvelope {
				hs := make([]Header, MaxHeaders+1)
				for i := range hs {
					hs[i] = hdr("X-N", "v")
				}
				return RequestEnvelope{Method: "GET", URL: u, Headers: hs}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 400 {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P19c fields 超 64 → 400",
			env: func(u string) RequestEnvelope {
				fs := make([]BodyField, MaxFields+1)
				for i := range fs {
					fs[i] = BodyField{Name: "f", Enabled: true, Type: "text"}
				}
				return RequestEnvelope{Method: "POST", URL: u, Body: Body{Type: "x-www-form-urlencoded", Fields: fs}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, _ *upcap) {
				if perr == nil || perr.HTTPStatus != 400 {
					t.Fatalf("perr=%v", perr)
				}
			},
		},
		{
			name: "P21 json 空 text → 无 body 无 CT",
			env: func(u string) RequestEnvelope {
				return RequestEnvelope{Method: "GET", URL: u, Body: Body{Type: "json", Text: ""}}
			},
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				_, h, body, _ := cap.get()
				if len(body) != 0 {
					t.Fatalf("不应有 body，得到 %q", body)
				}
				if ct := h.Get("Content-Type"); ct != "" {
					t.Fatalf("不应有 Content-Type，得到 %q", ct)
				}
			},
		},
		{
			name: "P20 HEAD 无 body",
			env:  func(u string) RequestEnvelope { return RequestEnvelope{Method: "HEAD", URL: u} },
			check: func(t *testing.T, _ Response, perr *ExecError, cap *upcap) {
				if perr != nil {
					t.Fatalf("perr: %v", perr)
				}
				m, _, body, _ := cap.get()
				if m != "HEAD" || len(body) != 0 {
					t.Fatalf("method=%s body=%q", m, body)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var (
				srv *httptest.Server
				cap *upcap
			)
			if tt.upstream != nil {
				srv, cap = tt.upstream()
			} else {
				cap = &upcap{}
				srv = httptest.NewServer(echoUpstream(cap))
			}
			defer srv.Close()

			resp, perr := Execute(tt.env(srv.URL), nil)
			tt.check(t, resp, perr, cap)
		})
	}
}

// TestExecuteTLSInsecure P15：自签 https，开关两种行为。
func TestExecuteTLSInsecure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("sec"))
	}))
	defer srv.Close()

	// 不开 insecure：TLS 校验失败 → 502。
	_, perr := Execute(RequestEnvelope{Method: "GET", URL: srv.URL}, nil)
	if perr == nil || perr.HTTPStatus != 502 {
		t.Fatalf("期望 502，得到 %v", perr)
	}

	// 开 insecure：成功。
	resp, perr := Execute(RequestEnvelope{
		Method:  "GET",
		URL:     srv.URL,
		Options: Options{InsecureSkipVerify: true},
	}, nil)
	if perr != nil {
		t.Fatalf("perr: %v", perr)
	}
	if resp.BodyText != "sec" {
		t.Fatalf("body=%q", resp.BodyText)
	}
}

// TestExecuteLimits 杂项校验（超时上限、binary 非法 base64、urlencoded 拒文件字段）。
func TestExecuteLimits(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	cases := []struct {
		name string
		env  RequestEnvelope
	}{
		{"timeoutMs 超上限", RequestEnvelope{Method: "GET", URL: srv.URL, Options: Options{TimeoutMs: 300_001}}},
		{"binary 非法 base64", RequestEnvelope{Method: "POST", URL: srv.URL, Body: Body{Type: "binary", Text: "@@@"}}},
		{"urlencoded 含文件字段", RequestEnvelope{Method: "POST", URL: srv.URL, Body: Body{Type: "x-www-form-urlencoded", Fields: []BodyField{{Name: "f", Type: "file", Enabled: true}}}}},
		{"body.type 非法", RequestEnvelope{Method: "POST", URL: srv.URL, Body: Body{Type: "weird"}}},
		{"method 非法", RequestEnvelope{Method: "GE T", URL: srv.URL}},
		{"url 缺 host", RequestEnvelope{Method: "GET", URL: "http://"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, perr := Execute(tc.env, nil)
			if perr == nil || perr.HTTPStatus != 400 {
				t.Fatalf("期望 400，得到 %v", perr)
			}
		})
	}
}
