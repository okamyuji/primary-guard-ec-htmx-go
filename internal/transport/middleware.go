package transport

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// requestIDKey ctx に request id を入れるためのキー型
type requestIDKey struct{}

// RequestID リクエスト ID を発番して ctx と response header に乗せる
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8)
		if _, err := rand.Read(buf); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		id := base64.RawURLEncoding.EncodeToString(buf)
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CurrentRequestID ctx から request id を取り出す
func CurrentRequestID(ctx context.Context) string {
	v := ctx.Value(requestIDKey{})
	if v == nil {
		return ""
	}
	id, ok := v.(string)
	if !ok {
		return ""
	}
	return id
}

// AccessLog 1 リクエスト 1 行のアクセスログを slog に出す
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http access",
				"req_id", CurrentRequestID(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recoverer panic を 500 に変換し、ログに出す
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"req_id", CurrentRequestID(r.Context()),
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					http.Error(w, "サーバ内部エラーが発生しました", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware リクエスト全体に timeout を掛ける
func TimeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "処理がタイムアウトしました")
	}
}

// statusRecorder ステータスコードと送信バイト数を覚える ResponseWriter ラッパ
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

// WriteHeader ステータスを記憶しつつ書き出す
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Write バイト数を集計しつつ書き出す
func (s *statusRecorder) Write(p []byte) (int, error) {
	n, err := s.ResponseWriter.Write(p)
	s.bytes += n
	return n, err
}
