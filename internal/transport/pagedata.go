// Package transport HTTP サーバ・ルーティング・中間ware・ハンドラの組み立てを行う
package transport

import (
	"context"
	"net/http"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
)

// PageData 全ページ共通でテンプレートに渡す土台
type PageData struct {
	Title     string
	UserID    int64
	IsAdmin   bool
	CSRFToken string
	Flash     *FlashMessage
}

// FlashMessage 一行ノティス
type FlashMessage struct {
	Kind    string
	Message string
}

// NewPageData ctx と引数から PageData を組み立てる
func NewPageData(ctx context.Context, title string, isAdmin bool) PageData {
	pd := PageData{Title: title}
	if uid, ok := auth.CurrentUserID(ctx); ok {
		pd.UserID = uid
	}
	if sess, ok := auth.CurrentSession(ctx); ok {
		pd.CSRFToken = sess.CSRFToken
	}
	pd.IsAdmin = isAdmin
	return pd
}

// WithFlash ユーザー向けの一時メッセージを乗せる
func (p PageData) WithFlash(kind, message string) PageData {
	p.Flash = &FlashMessage{Kind: kind, Message: message}
	return p
}

// requestCSRFToken POST フォーム検証後にレスポンス用のフィールドへ詰めるためのヘルパ
// セッションが無いリクエスト (ログイン前のフォーム) でも空文字を返さず、Cookie からトークンを発行する
func ensureCSRFToken(w http.ResponseWriter, r *http.Request, secure bool) (string, error) {
	token, err := auth.ReadCSRFCookie(r)
	if err == nil {
		return token, nil
	}
	return auth.IssueCSRFCookie(w, secure)
}
