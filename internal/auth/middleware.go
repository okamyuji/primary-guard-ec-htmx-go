package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// ctxKey context へ値を差し込むための未公開キー型
type ctxKey int

const (
	ctxKeyUserID ctxKey = iota + 1
	ctxKeySession
)

// UserLookup ユーザー ID から管理者フラグなどを取得する小さなインタフェース
type UserLookup interface {
	IsAdmin(ctx context.Context, userID int64) (bool, error)
}

// LoadSessionMiddleware セッション Cookie を読んで Find に成功したら ctx に注入する
// セッションが無い、または見つからない場合は ctx を変更しないだけで次に進む
func LoadSessionMiddleware(store SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := ReadSessionCookie(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := store.Find(r.Context(), id)
			if errors.Is(err, ErrSessionNotFound) {
				next.ServeHTTP(w, r)
				return
			}
			if err != nil {
				slog.Default().Warn("session find failed", "err", err)
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyUserID, sess.UserID)
			ctx = context.WithValue(ctx, ctxKeySession, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUser ログイン済みでなければ /login にリダイレクトする
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := CurrentUserID(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin 管理者でなければ 403 を返す
func RequireAdmin(users UserLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := CurrentUserID(r.Context())
			if !ok {
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}
			isAdmin, err := users.IsAdmin(r.Context(), uid)
			if err != nil {
				http.Error(w, "管理者判定に失敗しました", http.StatusInternalServerError)
				return
			}
			if !isAdmin {
				http.Error(w, "権限がありません", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CSRFProtect POST/PUT/DELETE などに対して CSRF トークンが一致しなければ 403 を返す
// GET と HEAD と OPTIONS は無条件で通す
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !VerifyCSRF(r) {
			http.Error(w, "CSRFトークンが一致しません", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CurrentUserID ctx から現在のユーザー ID を取り出す
// セッションが無いときは (0, false) を返す
func CurrentUserID(ctx context.Context) (int64, bool) {
	v := ctx.Value(ctxKeyUserID)
	if v == nil {
		return 0, false
	}
	uid, ok := v.(int64)
	if !ok || uid == 0 {
		return 0, false
	}
	return uid, true
}

// CurrentSession ctx から現在のセッションを取り出す
func CurrentSession(ctx context.Context) (Session, bool) {
	v := ctx.Value(ctxKeySession)
	if v == nil {
		return Session{}, false
	}
	sess, ok := v.(Session)
	if !ok || sess.ID == "" {
		return Session{}, false
	}
	return sess, true
}
