package cookies

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return u
}

// C1: Set→Cookies 基本回放。
func TestJarBasic(t *testing.T) {
	j := New()
	u := mustURL(t, "http://a.com/api/list")
	j.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "v1"}})
	got := j.Cookies(u)
	if len(got) != 1 || got[0].Name != "sid" || got[0].Value != "v1" {
		t.Fatalf("got %+v", got)
	}
}

// C2: 域匹配——.a.com 通配子域，不误发他域。
func TestJarDomainMatch(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{
		{Name: "wide", Value: "1", Domain: "a.com"},
	})
	if got := j.Cookies(mustURL(t, "https://x.a.com/api")); len(got) != 1 {
		t.Fatalf("子域应匹配，得到 %+v", got)
	}
	if got := j.Cookies(mustURL(t, "https://b.com/api")); len(got) != 0 {
		t.Fatalf("他域不应匹配，得到 %+v", got)
	}
}

// C3: 路径匹配——/api Cookie 只发 /api/*。
func TestJarPathMatch(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://a.com/api/login"), []*http.Cookie{
		{Name: "tk", Value: "1", Path: "/api"},
	})
	if got := j.Cookies(mustURL(t, "https://a.com/api/users")); len(got) != 1 {
		t.Fatalf("同前缀应匹配")
	}
	if got := j.Cookies(mustURL(t, "https://a.com/other")); len(got) != 0 {
		t.Fatalf("不同前缀不应匹配")
	}
}

// C4: Secure 仅 https 发送。
func TestJarSecure(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{{Name: "s", Value: "1", Secure: true}})
	if got := j.Cookies(mustURL(t, "https://a.com/")); len(got) != 1 {
		t.Fatalf("https 应发送 secure cookie")
	}
	if got := j.Cookies(mustURL(t, "http://a.com/")); len(got) != 0 {
		t.Fatalf("http 不应发送 secure cookie")
	}
}

// C5: 过期过滤 + 同（域,名,路径）覆盖。
func TestJarExpiryAndMerge(t *testing.T) {
	j := New()
	past := time.Now().Add(-time.Hour)
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{
		{Name: "old", Value: "1", Expires: past},
		{Name: "k", Value: "v1"},
	})
	if got := j.Cookies(mustURL(t, "https://a.com/")); len(got) != 1 || got[0].Value != "v1" {
		t.Fatalf("过期应过滤: %+v", got)
	}
	// 同名覆盖
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{{Name: "k", Value: "v2"}})
	if got := j.Cookies(mustURL(t, "https://a.com/")); len(got) != 1 || got[0].Value != "v2" {
		t.Fatalf("应覆盖: %+v", got)
	}
	if j.Len() != 1 {
		t.Fatalf("合并后应 1 条，得到 %d", j.Len())
	}
	// Max-Age 到期
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{{Name: "k", Value: "v3", MaxAge: -1}})
	if j.Len() != 0 {
		t.Fatalf("MaxAge<0 应即删（会话化后过期过滤）")
	}
}

// C6: 持久化往返 + Clear。
func TestJarPersist(t *testing.T) {
	j := New()
	j.SetCookies(mustURL(t, "https://a.com/"), []*http.Cookie{
		{Name: "sid", Value: "abc", Path: "/"},
		{Name: "s", Value: "1", Secure: true},
	})
	raw, err := j.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	j2 := New()
	if err := j2.Load(raw); err != nil {
		t.Fatalf("load: %v", err)
	}
	u := mustURL(t, "https://a.com/api")
	orig, got := j.Cookies(u), j2.Cookies(u)
	if len(orig) != len(got) || len(got) != 2 {
		t.Fatalf("往返应一致: orig=%d got=%d", len(orig), len(got))
	}
	for i := range orig {
		if orig[i].Name != got[i].Name || orig[i].Value != got[i].Value {
			t.Fatalf("cookie 不一致: %+v vs %+v", orig[i], got[i])
		}
	}
	j2.Clear()
	if j2.Len() != 0 {
		t.Fatalf("Clear 后应为空")
	}
}
