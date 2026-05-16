// Package main primary-guard-ec-htmx-go の HTTP サーバ起動エントリ
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

// main エントリポイント
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	logger.Info("primary-guard-ec-htmx-go listening", "addr", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// healthz 動作確認用のヘルスチェックハンドラ
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		slog.Default().Warn("healthz write failed", "err", err)
	}
}
