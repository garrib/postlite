// Package cookies 实现轻量 Cookie Jar（docs/design.md §3.3b）。
// 刻意不用 net/http/cookiejar：后者无法枚举已存 Cookie，做不了持久化；
// 这里的匹配规则是调试工具够用的简化子集（域后缀/路径前缀/Secure/过期）。
package cookies

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"` // 零值 = 会话 Cookie
	Secure   bool      `json:"secure,omitempty"`
	HttpOnly bool      `json:"httpOnly,omitempty"`
}

type fileFormat struct {
	Version int      `json:"version"`
	Cookies []cookie `json:"cookies"`
}

type Jar struct {
	mu      sync.Mutex
	cookies []cookie
	now     func() time.Time
}

func New() *Jar { return &Jar{now: time.Now} }

// SetCookies 实现 http.CookieJar：按（域, 名, 路径）三元组合并覆盖。
func (j *Jar) SetCookies(u *url.URL, incoming []*http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	host := strings.ToLower(u.Hostname())
	now := j.now()
	for _, c := range incoming {
		if c == nil || c.Name == "" {
			continue
		}
		domain := strings.ToLower(c.Domain)
		if domain == "" {
			domain = host
		}
		domain = strings.TrimPrefix(domain, ".")
		path := c.Path
		if path == "" {
			path = "/"
		}
		exp := c.Expires
		if c.MaxAge > 0 {
			exp = now.Add(time.Duration(c.MaxAge) * time.Second)
		} else if c.MaxAge < 0 {
			// RFC 6265：Max-Age<=0 为删除语义
			for i := range j.cookies {
				o := &j.cookies[i]
				if o.Domain == domain && o.Name == c.Name && o.Path == path {
					j.cookies = append(j.cookies[:i], j.cookies[i+1:]...)
					break
				}
			}
			continue
		}
		nc := cookie{
			Name: c.Name, Value: c.Value, Domain: domain, Path: path,
			Expires: exp, Secure: c.Secure, HttpOnly: c.HttpOnly,
		}
		replaced := false
		for i := range j.cookies {
			o := &j.cookies[i]
			if o.Domain == domain && o.Name == nc.Name && o.Path == path {
				j.cookies[i] = nc
				replaced = true
				break
			}
		}
		if !replaced {
			j.cookies = append(j.cookies, nc)
		}
	}
	j.cookies = filterExpired(j.cookies, now)
}

// Cookies 实现 http.CookieJar：返回应随 u 发送的 Cookie。
func (j *Jar) Cookies(u *url.URL) []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	host := strings.ToLower(u.Hostname())
	secure := u.Scheme == "https"
	path := u.Path
	if path == "" {
		path = "/"
	}
	var out []*http.Cookie
	for _, c := range filterExpired(j.cookies, j.now()) {
		if !domainMatch(host, c.Domain) || !strings.HasPrefix(path, c.Path) {
			continue
		}
		if c.Secure && !secure {
			continue
		}
		out = append(out, &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path})
	}
	return out
}

// Marshal 持久化导出；Load 导入（docs/data.md cookies.json）。
func (j *Jar) Marshal() ([]byte, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	raw, err := json.Marshal(fileFormat{Version: 1, Cookies: filterExpired(j.cookies, j.now())})
	return raw, err
}

func (j *Jar) Load(raw []byte) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	j.cookies = f.Cookies
	return nil
}

func (j *Jar) Clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies = nil
}

func (j *Jar) Len() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.cookies)
}

func domainMatch(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func filterExpired(all []cookie, now time.Time) []cookie {
	kept := all[:0]
	for _, c := range all {
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}
