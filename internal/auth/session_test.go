package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewSessionIDLength ID が 43 文字 base64url 表現になっていることを確認する
func TestNewSessionIDLength(t *testing.T) {
	t.Parallel()

	id, err := NewSessionID()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(id) != 43 {
		t.Fatalf("len got %d want 43", len(id))
	}
}

// TestIssueAndReadSessionCookie 書き出した Cookie を読み戻せること
func TestIssueAndReadSessionCookie(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	IssueSessionCookie(rec, "abc", time.Now().Add(time.Hour), false)

	res := rec.Result()
	if len(res.Cookies()) != 1 {
		t.Fatalf("cookies got %d want 1", len(res.Cookies()))
	}
	c := res.Cookies()[0]
	if c.Name != SessionCookieName {
		t.Errorf("name got %s want %s", c.Name, SessionCookieName)
	}
	if !c.HttpOnly {
		t.Errorf("HttpOnly want true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite want Lax")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	got, ok := ReadSessionCookie(req)
	if !ok || got != "abc" {
		t.Fatalf("read got (%s,%v) want (abc,true)", got, ok)
	}
}

// TestClearSessionCookie 空文字と MaxAge=-1 で失効させることを確認する
func TestClearSessionCookie(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	ClearSessionCookie(rec, false)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies got %d want 1", len(cookies))
	}
	if cookies[0].Value != "" {
		t.Errorf("value want empty got %s", cookies[0].Value)
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("MaxAge want -1 got %d", cookies[0].MaxAge)
	}
}

// TestReadSessionCookieAbsent 未設定で false を返す
func TestReadSessionCookieAbsent(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := ReadSessionCookie(req); ok {
		t.Fatal("ok want false")
	}
}
