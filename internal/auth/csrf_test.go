package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestIssueCSRFCookie Cookie 発行で SetCookie が呼ばれ、HttpOnly と SameSite=Lax が付くことを確認する
func TestIssueCSRFCookie(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	token, err := IssueCSRFCookie(rec, false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if token == "" {
		t.Fatal("token want non-empty")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies got %d want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != CSRFCookieName {
		t.Errorf("name got %s want %s", c.Name, CSRFCookieName)
	}
	if !c.HttpOnly {
		t.Errorf("HttpOnly want true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite want Lax")
	}
	if c.Path != "/" {
		t.Errorf("Path got %s want /", c.Path)
	}
}

// TestReadCSRFCookieMissing Cookie 未設定時に ErrCSRFMissing を返す
func TestReadCSRFCookieMissing(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, err := ReadCSRFCookie(req)
	if !errors.Is(err, ErrCSRFMissing) {
		t.Fatalf("err got %v want ErrCSRFMissing", err)
	}
}

// TestVerifyCSRFMatch Cookie とフォームのトークンが一致すれば true を返す
func TestVerifyCSRFMatch(t *testing.T) {
	t.Parallel()

	token, err := NewCSRFToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	form := url.Values{CSRFFormField: {token}}
	req := httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})

	if !VerifyCSRF(req) {
		t.Fatal("match want true")
	}
}

// TestVerifyCSRFMismatch 値が違えば false を返す
func TestVerifyCSRFMismatch(t *testing.T) {
	t.Parallel()

	form := url.Values{CSRFFormField: {"bad"}}
	req := httptest.NewRequest(http.MethodPost, "/x",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "good"})

	if VerifyCSRF(req) {
		t.Fatal("mismatch want false")
	}
}

// TestVerifyCSRFEmptyForm フォーム未入力で false
func TestVerifyCSRFEmptyForm(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "good"})

	if VerifyCSRF(req) {
		t.Fatal("empty form want false")
	}
}
