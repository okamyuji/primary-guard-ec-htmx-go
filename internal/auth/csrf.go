package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
)

const (
	// CSRFCookieName CSRF トークンを保存する Cookie 名
	CSRFCookieName = "pgec_csrf"
	// CSRFFormField フォーム内の CSRF 入力フィールド名
	CSRFFormField = "csrf"
	// CSRFTokenLen トークンのバイト長
	CSRFTokenLen = 32
)

// NewCSRFToken 新しい CSRF トークン (base64url) を生成する
func NewCSRFToken() (string, error) {
	buf := make([]byte, CSRFTokenLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("csrf rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// IssueCSRFCookie 新しいトークンを生成して Cookie に書き、その値を返す
// secure を true にすると Secure 属性を付ける
func IssueCSRFCookie(w http.ResponseWriter, secure bool) (string, error) {
	token, err := NewCSRFToken()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// ReadCSRFCookie Cookie から CSRF トークンを取り出す
// Cookie が無いか空のとき errors.Is(err, ErrCSRFMissing) で判定できる
func ReadCSRFCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return "", ErrCSRFMissing
	}
	if c.Value == "" {
		return "", ErrCSRFMissing
	}
	return c.Value, nil
}

// VerifyCSRF Cookie の値とフォーム入力の値が一致するかを定数時間で比較する
// 不一致のときは false を返す
func VerifyCSRF(r *http.Request) bool {
	cookieValue, err := ReadCSRFCookie(r)
	if err != nil {
		return false
	}
	formValue := r.PostFormValue(CSRFFormField)
	if formValue == "" {
		return false
	}
	if len(cookieValue) != len(formValue) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieValue), []byte(formValue)) == 1
}

// ErrCSRFMissing CSRF Cookie が存在しないことを表す
var ErrCSRFMissing = errors.New("csrf cookie missing")
