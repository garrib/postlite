package history

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/garrib/postlite/internal/proxy"
)

// fakeClock 可推进的假时钟（docs/test-cases.md §2）。
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) adv(d time.Duration) { c.t = c.t.Add(d) }

func baseEnv() proxy.RequestEnvelope {
	return proxy.RequestEnvelope{Method: "GET", URL: "http://127.0.0.1:1/x"}
}

func newStore(t *testing.T, dir string, clk *fakeClock) *Store {
	t.Helper()
	s, err := Open(dir, clk.now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

// TestAppendListOrder H1：追加后 List 降序。
func TestAppendListOrder(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s := newStore(t, t.TempDir(), clk)

	for i := 0; i < 3; i++ {
		if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
			t.Fatalf("Append: %v", err)
		}
		clk.adv(time.Minute)
	}

	got := s.List()
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].CreatedAt < got[1].CreatedAt || got[1].CreatedAt < got[2].CreatedAt {
		t.Fatalf("应降序: %d %d %d", got[0].CreatedAt, got[1].CreatedAt, got[2].CreatedAt)
	}
}

// TestRetentionBoundary H2：7 天边界，前过期后保留。
func TestRetentionBoundary(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	dir := t.TempDir()
	s := newStore(t, dir, clk)

	oldEnv := baseEnv()
	oldEnv.URL = "http://old"
	if err := s.Append(NewRecord(clk.now(), oldEnv, nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// 前进 7 天整（createdAt == cutoff，不小于，应保留）。
	clk.adv(7*24*time.Hour - time.Millisecond)
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	clk.adv(2 * time.Millisecond)
	// 边界条目：createdAt = now-7d-1ms → 应被清掉；newest 保留。
	_ = s.List()
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got := s.List()
	if len(got) != 2 {
		t.Fatalf("应剩 2 条，得到 %d：%+v", len(got), got)
	}
	for _, r := range got {
		if r.Request.URL == "http://old" {
			t.Fatalf("最老条目应已过期清理")
		}
	}
}

// TestPruneOnStartup H3：启动即清理过期条目。
func TestPruneOnStartup(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}

	s := newStore(t, dir, clk)
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// 时间前进 8 天后重新打开。
	clk.adv(8 * 24 * time.Hour)
	s2, err := Open(dir, clk.now)
	if err != nil {
		t.Fatalf("重开: %v", err)
	}
	if got := s2.List(); len(got) != 0 {
		t.Fatalf("启动应清理过期历史，剩 %d 条", len(got))
	}

	// 文件里也不应再有该条目。
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("文件不是合法 JSON: %v", err)
	}
	if len(f.Items) != 0 {
		t.Fatalf("文件应无条目，剩 %d", len(f.Items))
	}
}

// TestMaxItems H4：10000 条上限，从最旧端丢（白盒直填，避免万次落盘）。
func TestMaxItems(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s := newStore(t, t.TempDir(), clk)

	s.mu.Lock()
	s.items = make([]Record, maxItems+5)
	for i := range s.items {
		s.items[i] = NewRecord(clk.now().Add(time.Duration(i)*time.Second), baseEnv(), nil, "")
	}
	// 记住追加后应成为队首的条目（原第 6 旧，前 5 条会被挤掉，再被 Append 挤掉 1 条）。
	expectFirst := s.items[6].CreatedAt
	s.mu.Unlock()

	if err := s.Append(NewRecord(clk.now().Add(time.Hour), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) != maxItems {
		t.Fatalf("应裁到 %d，得到 %d", maxItems, len(s.items))
	}
	if s.items[0].CreatedAt != expectFirst {
		t.Fatalf("最旧端丢弃数不对: 队首 createdAt=%d，期望 %d", s.items[0].CreatedAt, expectFirst)
	}
}

// TestCorruptRecovery H5：损坏文件归档、空表启动、进程不崩。
func TestCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	garbage := []byte(`{"version":1,"items":[ `)
	if err := os.WriteFile(path, garbage, 0o600); err != nil {
		t.Fatalf("写损坏文件: %v", err)
	}

	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s, err := Open(dir, clk.now)
	if err != nil {
		t.Fatalf("损坏文件不应导致 Open 失败: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("应为空，得到 %d", len(got))
	}

	// 原件应被归档且内容不变。
	matches, _ := filepath.Glob(path + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatalf("应有一个归档文件，得到 %v", matches)
	}
	archived, _ := os.ReadFile(matches[0])
	if string(archived) != string(garbage) {
		t.Fatalf("归档内容与原件不符")
	}

	// 归档后应能正常继续使用。
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if got := s.List(); len(got) != 1 {
		t.Fatalf("应 1 条，得到 %d", len(got))
	}
}

// TestDeleteMissing H6：删除不存在 id 幂等。
func TestDeleteMissing(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	dir := t.TempDir()
	s := newStore(t, dir, clk)
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Delete("no-such-id"); err != nil {
		t.Fatalf("删除不存在 id 不应报错: %v", err)
	}
	if got := s.List(); len(got) != 1 {
		t.Fatalf("原有条目不应受影响，得到 %d", len(got))
	}

	// 正常删除。
	id := s.List()[0].ID
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("应已删除，剩 %d", len(got))
	}
}

// TestClear H7：清空落盘。
func TestClear(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s := newStore(t, dir, clk)
	for i := 0; i < 3; i++ {
		if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := s.List(); len(got) != 0 {
		t.Fatalf("应已清空，剩 %d", len(got))
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil || len(f.Items) != 0 {
		t.Fatalf("文件应 items=[]，raw=%s err=%v", raw, err)
	}
}

// TestSaveAtomicity H8：Append 后文件是完整合法 JSON 且与内存一致。
func TestSaveAtomicity(t *testing.T) {
	dir := t.TempDir()
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s := newStore(t, dir, clk)
	env := baseEnv()
	env.URL = "http://x/y"
	if err := s.Append(NewRecord(clk.now(), env, &proxy.Response{Status: 200, IsText: true, BodyText: "ok"}, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("读文件: %v", err)
	}
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("原子写后应为合法 JSON: %v", err)
	}
	if f.Version != fileVersion || len(f.Items) != 1 {
		t.Fatalf("f=%+v", f)
	}
	if f.Items[0].Response.Status != 200 || f.Items[0].Request.URL != "http://x/y" {
		t.Fatalf("落盘内容不符: %+v", f.Items[0])
	}
}

// TestPreviewTruncate H9：预览 256KB、rune 边界安全。
func TestPreviewTruncate(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	// 构造 25 万个“好”字（750KB），保证跨过 256KB 边界且边界处是多字节字符。
	big := strings.Repeat("好", 250_000)
	env := baseEnv()
	rec := NewRecord(clk.now(), env, &proxy.Response{Status: 200, IsText: true, BodyText: big}, "")
	sum := rec.Response
	if !sum.PreviewTruncated {
		t.Fatalf("应标记截断")
	}
	if len(sum.PreviewText) > previewMaxBytes {
		t.Fatalf("预览 %d 字节 > %d", len(sum.PreviewText), previewMaxBytes)
	}
	// rune 边界：不会以半个字符结尾。
	if !utf8.ValidString(sum.PreviewText) {
		t.Fatalf("预览应以完整 UTF-8 字符结尾")
	}

	// 小响应不截断。
	rec2 := NewRecord(clk.now(), env, &proxy.Response{Status: 200, IsText: true, BodyText: "abc"}, "")
	if rec2.Response.PreviewTruncated || rec2.Response.PreviewText != "abc" {
		t.Fatalf("小响应不应截断: %+v", rec2.Response)
	}
}

// TestBigFilePlaceholder H10：>1MB 文件内容落历史丢弃、占位保留。
func TestBigFilePlaceholder(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	bigRaw := strings.Repeat("z", fileKeepMaxRaw+1)
	env := baseEnv()
	env.Body = proxy.Body{
		Type: "form-data",
		Fields: []proxy.BodyField{
			{Name: "f", Type: "file", Enabled: true, FileName: "big.bin", ContentBase64: base64.StdEncoding.EncodeToString([]byte(bigRaw))},
			{Name: "k", Type: "text", Enabled: true, Value: "v"},
		},
	}
	rec := NewRecord(clk.now(), env, nil, "")
	if rec.Request.Body.Fields[0].ContentBase64 != "" {
		t.Fatalf("大文件内容应被丢弃留占位")
	}
	if rec.Request.Body.Fields[0].FileName != "big.bin" {
		t.Fatalf("占位元数据应保留")
	}
	if rec.Request.Body.Fields[1].Value != "v" {
		t.Fatalf("文本字段不受影响")
	}
}

// TestRecordErrField 发送失败历史带 error。
func TestRecordErrField(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	rec := NewRecord(clk.now(), baseEnv(), nil, "连接拒绝")
	if rec.Error != "连接拒绝" || rec.Response != nil {
		t.Fatalf("rec=%+v", rec)
	}
}
