// Package main outbox イベントを Primary から取り出して処理するワーカーの起動エントリ
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/config"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/outbox"
)

// main エントリポイント
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped with error", "err", err)
		os.Exit(1)
	}
}

// run config 読み込みから worker 起動と shutdown までを行う
func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	primary, err := openPrimary(cfg.PrimaryDSN, logger)
	if err != nil {
		return err
	}
	defer obs.CloseAndLog(primary, "primary db")

	router := dbx.New(primary, primary, nil)
	store := outbox.NewStore(router)
	worker := outbox.NewWorker(store, outbox.LogHandler(logger), cfg.OutboxInterval, cfg.OutboxBatch, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, stopping worker", "signal", sig.String())
		cancel()
	}()

	worker.Run(ctx)
	return nil
}

// openPrimary Primary 用に DSN を開き、connection pool 設定を適用する
func openPrimary(dsn string, logger *slog.Logger) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	dbx.ConfigurePool(db, dbx.DefaultPoolConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		obs.CloseAndLog(db, "primary db ping failed")
		return nil, err
	}
	logger.Info("primary opened")
	return db, nil
}
