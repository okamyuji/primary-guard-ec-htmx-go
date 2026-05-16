package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// SessionCookieName セッション ID を保存する Cookie 名
	SessionCookieName = "pgec_sid"
	// SessionIDByteLen セッション ID のバイト長 (base64url で 43 文字)
	SessionIDByteLen = 32
)

// Session セッションの値オブジェクト
type Session struct {
	ID        string
	UserID    int64
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// SessionStore セッションの保存先を表すインタフェース
// DB 実装は internal/auth/sessionstore.go に置く
type SessionStore interface {
	Create(ctx context.Context, userID int64, ttl time.Duration, csrfToken string) (Session, error)
	Find(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// ErrSessionNotFound セッションが見つからないことを表す
var ErrSessionNotFound = errors.New("session not found")

// NewSessionID 暗号学的に安全なセッション ID を生成する
func NewSessionID() (string, error) {
	buf := make([]byte, SessionIDByteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("session rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// IssueSessionCookie セッション Cookie を書き出す
// expires にはセッションの有効期限を渡す
func IssueSessionCookie(w http.ResponseWriter, id string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    id,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie セッション Cookie を即時に失効させる
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ReadSessionCookie リクエスト Cookie からセッション ID を読み出す
// Cookie が無いか空のとき false を返す
func ReadSessionCookie(r *http.Request) (string, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", false
	}
	if c.Value == "" {
		return "", false
	}
	return c.Value, true
}
