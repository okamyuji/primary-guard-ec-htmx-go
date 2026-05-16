package transport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/static"
)

// Server アプリ全体のミドルウェアとルーティングをまとめた http.Handler を返す
type Server struct {
	Auth     *AuthDeps
	Catalog  *CatalogDeps
	Cart     *CartDeps
	Order    *OrderDeps
	Report   *ReportDeps
	Sessions auth.SessionStore
	Users    auth.UserLookup
	Logger   *slog.Logger
}

// NewServer 標準ライブラリの http.Server を組み立てて返す
// addr が空文字なら :8080 を使う
func (s *Server) NewServer(addr string) *http.Server {
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS())))

	// 公開ページ
	mux.HandleFunc("GET /", s.Catalog.Home)
	mux.HandleFunc("GET /products/{id}", s.Catalog.Detail)
	mux.HandleFunc("GET /suggest", s.Catalog.Suggest)

	mux.HandleFunc("GET /login", s.Auth.LoginPage)
	mux.Handle("POST /login", auth.CSRFProtect(http.HandlerFunc(s.Auth.LoginSubmit)))
	mux.HandleFunc("GET /register", s.Auth.RegisterPage)
	mux.Handle("POST /register", auth.CSRFProtect(http.HandlerFunc(s.Auth.RegisterSubmit)))
	mux.HandleFunc("GET /admin/login", s.Auth.AdminLoginPage)
	mux.Handle("POST /admin/login", auth.CSRFProtect(http.HandlerFunc(s.Auth.AdminLoginSubmit)))
	mux.Handle("POST /logout", auth.CSRFProtect(http.HandlerFunc(s.Auth.Logout)))

	// 要ログイン
	mux.Handle("GET /cart", auth.RequireUser(http.HandlerFunc(s.Cart.View)))
	mux.Handle("POST /cart/add", auth.RequireUser(auth.CSRFProtect(http.HandlerFunc(s.Cart.Add))))
	mux.Handle("POST /cart/update", auth.RequireUser(auth.CSRFProtect(http.HandlerFunc(s.Cart.Update))))
	mux.Handle("POST /cart/remove", auth.RequireUser(auth.CSRFProtect(http.HandlerFunc(s.Cart.Remove))))
	mux.Handle("POST /orders", auth.RequireUser(auth.CSRFProtect(http.HandlerFunc(s.Order.Create))))
	mux.Handle("GET /orders/{id}", auth.RequireUser(http.HandlerFunc(s.Order.Detail)))
	mux.Handle("GET /orders", auth.RequireUser(http.HandlerFunc(s.Order.History)))

	// 要管理者
	mux.Handle("GET /admin/report", auth.RequireAdmin(s.Users)(http.HandlerFunc(s.Report.Monthly)))

	// 共通ミドルウェアチェーン
	var handler http.Handler = mux
	handler = auth.LoadSessionMiddleware(s.Sessions)(handler)
	handler = Recoverer(s.Logger)(handler)
	handler = AccessLog(s.Logger)(handler)
	handler = RequestID(handler)

	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// healthz 動作確認用のヘルスチェック
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	obs.WriteAndLog(w, []byte("ok"), "healthz")
}
