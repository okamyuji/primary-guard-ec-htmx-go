package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeStore SessionStore のテスト用フェイク実装
type fakeStore struct {
	sessions map[string]Session
	err      error
}

func (f *fakeStore) Create(_ context.Context, userID int64, ttl time.Duration, token string) (Session, error) {
	id, _ := NewSessionID()
	s := Session{
		ID:        id,
		UserID:    userID,
		CSRFToken: token,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if f.sessions == nil {
		f.sessions = map[string]Session{}
	}
	f.sessions[id] = s
	return s, nil
}

func (f *fakeStore) Find(_ context.Context, id string) (Session, error) {
	if f.err != nil {
		return Session{}, f.err
	}
	s, ok := f.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return s, nil
}

func (f *fakeStore) Delete(_ context.Context, id string) error {
	delete(f.sessions, id)
	return nil
}

func (f *fakeStore) DeleteExpired(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// fakeUsers UserLookup のフェイク
type fakeUsers struct {
	admins map[int64]bool
	err    error
}

func (f *fakeUsers) IsAdmin(_ context.Context, uid int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.admins[uid], nil
}

// TestLoadSessionMiddlewareInjectsUserID Cookie とセッションが揃えば ctx に UserID が入る
func TestLoadSessionMiddlewareInjectsUserID(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	s, _ := store.Create(context.Background(), 7, time.Hour, "tok")

	gotUID := int64(0)
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		uid, _ := CurrentUserID(r.Context())
		gotUID = uid
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: s.ID})
	rec := httptest.NewRecorder()
	LoadSessionMiddleware(store)(next).ServeHTTP(rec, req)

	if gotUID != 7 {
		t.Fatalf("uid got %d want 7", gotUID)
	}
}

// TestLoadSessionMiddlewarePassesThroughOnError Find エラーでも次に進む
func TestLoadSessionMiddlewarePassesThroughOnError(t *testing.T) {
	t.Parallel()

	store := &fakeStore{err: errors.New("boom")}
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "x"})
	rec := httptest.NewRecorder()
	LoadSessionMiddleware(store)(next).ServeHTTP(rec, req)

	if !called {
		t.Fatal("next not called")
	}
}

// TestRequireUserRedirectsAnonymous 未認証は /login にリダイレクト
func TestRequireUserRedirectsAnonymous(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status got %d want 302", rec.Code)
	}
	if rec.Header().Get("Location") != "/login" {
		t.Fatalf("location got %s want /login", rec.Header().Get("Location"))
	}
}

// TestRequireAdminRejectsNonAdmin 一般ユーザーは 403
func TestRequireAdminRejectsNonAdmin(t *testing.T) {
	t.Parallel()

	users := &fakeUsers{admins: map[int64]bool{}}
	ctx := context.WithValue(context.Background(), ctxKeyUserID, int64(9))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/report", nil).WithContext(ctx)
	RequireAdmin(users)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want 403", rec.Code)
	}
}

// TestRequireAdminAllowsAdmin 管理者は通す
func TestRequireAdminAllowsAdmin(t *testing.T) {
	t.Parallel()

	users := &fakeUsers{admins: map[int64]bool{1: true}}
	ctx := context.WithValue(context.Background(), ctxKeyUserID, int64(1))

	called := false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/report", nil).WithContext(ctx)
	RequireAdmin(users)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(rec, req)

	if !called {
		t.Fatal("next not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want 200", rec.Code)
	}
}

// TestCSRFProtectAllowsGet GET は無条件に通す
func TestCSRFProtectAllowsGet(t *testing.T) {
	t.Parallel()

	called := false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	CSRFProtect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})).ServeHTTP(rec, req)

	if !called {
		t.Fatal("next not called")
	}
}

// TestCSRFProtectRejectsPostMismatch POST かつ不一致で 403
func TestCSRFProtectRejectsPostMismatch(t *testing.T) {
	t.Parallel()

	form := url.Values{CSRFFormField: {"a"}}
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "b"})
	rec := httptest.NewRecorder()
	CSRFProtect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next must not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want 403", rec.Code)
	}
}
