package transport

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/user"
)

// AuthUsers ログイン処理側に必要なユーザー操作だけを表す
type AuthUsers interface {
	Authenticate(ctx context.Context, email, password string) (domain.User, error)
	Register(ctx context.Context, email, password string, isAdmin bool) (int64, error)
}

// AuthDeps 認証ハンドラの依存
// 具体型を直接は持たず interface だけを受ける
type AuthDeps struct {
	Renderer     *render.Renderer
	Users        AuthUsers
	Sessions     auth.SessionStore
	SecureCookie bool
	SessionTTL   time.Duration
}

// LoginPage ログイン画面 (GET)
func (d *AuthDeps) LoginPage(w http.ResponseWriter, r *http.Request) {
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "ログイン", false)
	pd.CSRFToken = token
	if err := d.Renderer.Page(w, "login.html", pd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// LoginSubmit ログイン処理 (POST)
func (d *AuthDeps) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	u, err := d.Users.Authenticate(r.Context(), email, password)
	if err != nil {
		d.renderLoginError(w, r, "メールまたはパスワードが正しくありません")
		return
	}
	d.startSession(w, r, u.ID, "/")
}

// RegisterPage 新規登録画面 (GET)
func (d *AuthDeps) RegisterPage(w http.ResponseWriter, r *http.Request) {
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "新規登録", false)
	pd.CSRFToken = token
	if err := d.Renderer.Page(w, "register.html", pd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// RegisterSubmit 新規登録処理 (POST)
func (d *AuthDeps) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	uid, err := d.Users.Register(r.Context(), email, password, false)
	if errors.Is(err, user.ErrEmailTaken) {
		d.renderRegisterError(w, r, "そのメールアドレスは既に登録されています")
		return
	}
	if err != nil {
		d.renderRegisterError(w, r, "登録に失敗しました")
		return
	}
	d.startSession(w, r, uid, "/")
}

// Logout セッションを破棄して / にリダイレクト
func (d *AuthDeps) Logout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := auth.CurrentSession(r.Context()); ok {
		if err := d.Sessions.Delete(r.Context(), sess.ID); err != nil {
			http.Error(w, "ログアウトに失敗しました", http.StatusInternalServerError)
			return
		}
	}
	auth.ClearSessionCookie(w, d.SecureCookie)
	http.Redirect(w, r, "/", http.StatusFound)
}

// AdminLoginPage 管理者ログイン画面 (GET)
func (d *AuthDeps) AdminLoginPage(w http.ResponseWriter, r *http.Request) {
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "管理者ログイン", false)
	pd.CSRFToken = token
	if err := d.Renderer.Page(w, "admin_login.html", pd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// AdminLoginSubmit 管理者ログイン処理 (POST)
func (d *AuthDeps) AdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	u, err := d.Users.Authenticate(r.Context(), email, password)
	if err != nil || !u.IsAdmin {
		http.Error(w, "管理者として認証できません", http.StatusUnauthorized)
		return
	}
	d.startSession(w, r, u.ID, "/admin/report")
}

// startSession セッションを発行し、Cookie に書き、redirect する
func (d *AuthDeps) startSession(w http.ResponseWriter, r *http.Request, userID int64, redirectTo string) {
	token, err := auth.NewCSRFToken()
	if err != nil {
		http.Error(w, "セッション初期化に失敗しました", http.StatusInternalServerError)
		return
	}
	sess, err := d.Sessions.Create(r.Context(), userID, d.SessionTTL, token)
	if err != nil {
		http.Error(w, "セッション保存に失敗しました", http.StatusInternalServerError)
		return
	}
	auth.IssueSessionCookie(w, sess.ID, sess.ExpiresAt, d.SecureCookie)
	if _, err := auth.IssueCSRFCookie(w, d.SecureCookie); err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// renderLoginError エラー付きでログイン画面を再描画する
func (d *AuthDeps) renderLoginError(w http.ResponseWriter, r *http.Request, msg string) {
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "ログイン", false).WithFlash("error", msg)
	pd.CSRFToken = token
	w.WriteHeader(http.StatusUnauthorized)
	if err := d.Renderer.Page(w, "login.html", pd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// renderRegisterError エラー付きで新規登録画面を再描画する
func (d *AuthDeps) renderRegisterError(w http.ResponseWriter, r *http.Request, msg string) {
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "新規登録", false).WithFlash("error", msg)
	pd.CSRFToken = token
	w.WriteHeader(http.StatusBadRequest)
	if err := d.Renderer.Page(w, "register.html", pd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
