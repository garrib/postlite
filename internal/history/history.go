// Package history 实现 7 天保留的本地 JSON 历史，数据契约见 docs/data.md。
package history

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/garrib/postlite/internal/proxy"
)

const (
	retention       = 7 * 24 * time.Hour // 保留窗口（design.md §3.3）
	maxItems        = 10_000             // 数量上限
	previewMaxBytes = 256 << 10          // 文本预览上限
	fileKeepMaxRaw  = 1 << 20            // 历史中单文件字段内容上限，超出丢弃内容只留占位
	fileVersion     = 1
	FileName        = "history.json"
)

// ResponseSummary 历史中的响应摘要（不含响应体本体，只有预览）。
type ResponseSummary struct {
	Status           int    `json:"status"`
	StatusText       string `json:"statusText"`
	SizeBytes        int64  `json:"sizeBytes"`
	TimeMs           int64  `json:"timeMs"`
	ContentType      string `json:"contentType"`
	PreviewText      string `json:"previewText,omitempty"`
	PreviewTruncated bool   `json:"previewTruncated,omitempty"`
}

// Record 一条历史（docs/data.md §2）。
type Record struct {
	ID        string                `json:"id"`
	CreatedAt int64                 `json:"createdAt"`
	Request   proxy.RequestEnvelope `json:"request"`
	Response  *ResponseSummary      `json:"response,omitempty"`
	Error     string                `json:"error,omitempty"`
}

type fileFormat struct {
	Version int      `json:"version"`
	Items   []Record `json:"items"`
}

// Store 内存为主、文件为真源的存储；并发安全。
type Store struct {
	mu    sync.Mutex
	path  string
	items []Record // createdAt 升序
	now   func() time.Time
}

// Open 加载（或新建）存储；文件损坏时归档原件后从空开始（data.md §4）。
func Open(dir string, now ...func() time.Time) (*Store, error) {
	path := filepath.Join(dir, FileName)
	s := &Store{path: path, now: time.Now}
	if len(now) > 0 && now[0] != nil {
		s.now = now[0]
	}

	raw, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("读取历史失败: %w", err)
	}

	var f fileFormat
	if uerr := json.Unmarshal(raw, &f); uerr != nil {
		// 损坏：归档原件（不删），从空开始。
		archive := path + ".corrupt-" + strconv.FormatInt(s.now().UnixMilli(), 10)
		if rerr := os.Rename(path, archive); rerr != nil {
			return nil, fmt.Errorf("历史文件损坏且无法归档: %w", rerr)
		}
		log.Printf("postlite: 历史文件损坏，原件已归档为 %s", archive)
		return s, nil
	}

	// 防御：落盘前按时间排序，加载后亦保证升序不变量。
	sort.SliceStable(f.Items, func(i, j int) bool { return f.Items[i].CreatedAt < f.Items[j].CreatedAt })
	s.items = f.Items
	s.pruneLocked() // 启动即清理（design.md §3.3）
	if serr := s.saveLocked(); serr != nil {
		return nil, serr
	}
	return s, nil
}

// NewRecord 生成一条历史；resp 与 errMsg 互斥，二选一传。
func NewRecord(now time.Time, req proxy.RequestEnvelope, resp *proxy.Response, errMsg string) Record {
	rec := Record{
		ID:        newID(now),
		CreatedAt: now.UnixMilli(),
		Request:   sanitizeRequest(req),
		Error:     errMsg,
	}
	if resp != nil {
		sum := &ResponseSummary{
			Status:     resp.Status,
			StatusText: resp.StatusText,
			SizeBytes:  resp.SizeBytes,
			TimeMs:     resp.TimeMs,
		}
		for _, h := range resp.Headers {
			if h.Name == "Content-Type" {
				sum.ContentType = h.Value
				break
			}
		}
		if resp.IsText {
			sum.PreviewText, sum.PreviewTruncated = truncateRunes(resp.BodyText, previewMaxBytes)
		}
		rec.Response = sum
	}
	return rec
}

// Append 追加一条历史（先清理再落盘，Append 自身生成 id/createdAt 由调用方通过 NewRecord 提供）。
func (s *Store) Append(rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.items = append(s.items, rec)
	if excess := len(s.items) - maxItems; excess > 0 {
		s.items = s.items[excess:]
	}
	return s.saveLocked()
}

// List 返回降序副本。
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.items))
	copy(out, s.items)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Delete 删除一条（幂等：不存在也成功）。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.items {
		if r.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.saveLocked()
		}
	}
	return nil
}

// Clear 清空全部。
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
	return s.saveLocked()
}

// pruneLocked 惰性清理：只丢 7 天前的最老条目（越界即停，不重排未过期数据）。
func (s *Store) pruneLocked() {
	cutoff := s.now().Add(-retention).UnixMilli()
	drop := 0
	for drop < len(s.items) && s.items[drop].CreatedAt < cutoff {
		drop++
	}
	if drop > 0 {
		s.items = s.items[drop:]
	}
}

// saveLocked 原子落盘：临时文件 + rename（data.md §1）。
func (s *Store) saveLocked() error {
	raw, err := json.Marshal(fileFormat{Version: fileVersion, Items: s.items})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name) //nolint:errcheck // rename 成功后 Remove 是 no-op
	if err = tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}
	if _, err = tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err = os.Rename(name, s.path); err != nil {
		return fmt.Errorf("替换历史文件失败: %w", err)
	}
	return nil
}

// sanitizeRequest 落历史前的瘦身：>1MB 原始内容的文件字段丢弃内容只留占位（data.md §2）。
func sanitizeRequest(req proxy.RequestEnvelope) proxy.RequestEnvelope {
	req.Record = nil // 历史里的信封不再携带 record 开关
	for i := range req.Body.Fields {
		f := &req.Body.Fields[i]
		if f.ContentBase64 == "" {
			continue
		}
		if raw, err := base64.StdEncoding.DecodeString(f.ContentBase64); err != nil || len(raw) > fileKeepMaxRaw {
			f.ContentBase64 = ""
		}
	}
	return req
}

// newID 创建时刻毫秒（36 进制）+ "-" + 4 位随机 hex。
func newID(t time.Time) string {
	var buf [2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败极罕见；退化用时间低位保证唯一性倾向。
		b := uint8(t.UnixNano())
		buf[0], buf[1] = b, ^b
	}
	return strconv.FormatInt(t.UnixMilli(), 36) + "-" + hex.EncodeToString(buf[:])
}

// truncateRunes 按字节预算截到 rune 边界。
func truncateRunes(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	end := 0
	for end < limit {
		_, size := utf8.DecodeRuneInString(s[end:])
		if size == 0 || end+size > limit {
			break
		}
		end += size
	}
	return s[:end], true
}
