package history

import (
	"testing"
	"time"
)

// H11: TogglePin 翻转 + List 置顶。
func TestTogglePin(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	dir := t.TempDir()
	s := newStore(t, dir, clk)

	a := NewRecord(clk.now(), baseEnv(), nil, "")
	if err := s.Append(a); err != nil {
		t.Fatalf("Append: %v", err)
	}
	clk.adv(time.Minute)
	b := NewRecord(clk.now(), baseEnv(), nil, "")
	if err := s.Append(b); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// 钉选旧条目 a → List 应 a 在前
	pinned, err := s.TogglePin(a.ID)
	if err != nil || !pinned {
		t.Fatalf("TogglePin: %v %v", pinned, err)
	}
	got := s.List()
	if got[0].ID != a.ID || !got[0].Pinned {
		t.Fatalf("钉选应置顶: %+v", got[0])
	}

	// 再翻转取消
	if pinned, err := s.TogglePin(a.ID); err != nil || pinned {
		t.Fatalf("再翻转应 false: %v %v", pinned, err)
	}
	// 不存在 id 报错
	if _, err := s.TogglePin("nope"); err == nil {
		t.Fatalf("不存在 id 应报错")
	}
}

// H12: 钉选豁免 7 天清理。
func TestPinExemptFromRetention(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	dir := t.TempDir()
	s := newStore(t, dir, clk)

	old := NewRecord(clk.now(), baseEnv(), nil, "")
	if err := s.Append(old); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := s.TogglePin(old.ID); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// 前进 8 天，追加触发清理
	clk.adv(8 * 24 * time.Hour)
	if err := s.Append(NewRecord(clk.now(), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got := s.List()
	if len(got) != 2 {
		t.Fatalf("钉选条目应豁免清理，得到 %d 条", len(got))
	}
	if got[0].ID != old.ID {
		t.Fatalf("钉选应置顶")
	}
}

// H13: 数量上限淘汰顺序——最旧未钉选优先，全钉选才动最旧钉选。
func TestPinCapOrder(t *testing.T) {
	clk := &fakeClock{t: time.UnixMilli(1732531200000)}
	s := newStore(t, t.TempDir(), clk)

	// 直接白盒填 maxItems 条（免万次落盘），最旧 3 条钉选
	s.mu.Lock()
	s.items = make([]Record, maxItems)
	for i := range s.items {
		s.items[i] = NewRecord(clk.now().Add(time.Duration(i)*time.Second), baseEnv(), nil, "")
	}
	for i := 0; i < 3; i++ {
		s.items[i].Pinned = true
	}
	s.mu.Unlock()

	// 追加 2 条 → 挤掉 2 条最旧未钉选（即原 index 3、4），钉选保留
	if err := s.Append(NewRecord(clk.now().Add(time.Hour), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Append(NewRecord(clk.now().Add(2*time.Hour), baseEnv(), nil, "")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) != maxItems {
		t.Fatalf("应维持上限 %d，得到 %d", maxItems, len(s.items))
	}
	// 3 条钉选必须都在
	pinned := 0
	for _, r := range s.items {
		if r.Pinned {
			pinned++
		}
	}
	if pinned != 3 {
		t.Fatalf("钉选应保留 3 条，得到 %d", pinned)
	}
	// 最旧的未钉选（index 3 的 createdAt）应已被淘汰
	gone := clk.now().Add(3 * time.Second).UnixMilli()
	for _, r := range s.items {
		if r.CreatedAt == gone && !r.Pinned {
			t.Fatalf("最旧未钉选应被淘汰，仍存在 createdAt=%d", gone)
		}
	}
}
