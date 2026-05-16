// Package main primary-guard-ec-htmx-go の HTTP サーバ起動エントリ
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/cart"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/catalog"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/config"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/inventory"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/migrate"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/order"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/outbox"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/report"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/transport"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/user"
)

// main エントリポイント
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped with error", "err", err)
		os.Exit(1)
	}
}

// run 設定読み込みから shutdown までを一気通貫で実行する
func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	primary, err := openDB(cfg.PrimaryDSN, "primary", logger)
	if err != nil {
		return err
	}
	defer obs.CloseAndLog(primary, "primary db")

	replica, err := openDB(cfg.ReplicaDSN, "replica", logger)
	if err != nil {
		return err
	}
	defer obs.CloseAndLog(replica, "replica db")

	migrateCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := migrate.Apply(migrateCtx, primary, cfg.MigrationsDir); err != nil {
		return err
	}
	logger.Info("migrations applied", "dir", cfg.MigrationsDir)

	state := dbx.NewReplicaState()
	router := dbx.New(primary, replica, state)

	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	health := dbx.NewReplicaHealth(replica, state, cfg.ReplicaHealthInterval, cfg.ReplicaLagTripAfter, logger)
	go health.Run(healthCtx)

	sessionStore := auth.NewDBSessionStore(primary)
	userRepo := user.NewRepository(router)
	catalogRepo := catalog.NewRepository(router)
	cartRepo := cart.NewRepository(router)
	inventoryRepo := inventory.NewRepository()
	outboxStore := outbox.NewStore(router)
	orderRepo := order.NewRepository(router, cartRepo, inventoryRepo, outboxStore)
	reportRepo := report.NewRepository(router)

	renderer, err := render.New()
	if err != nil {
		return err
	}
	categoryCache := catalog.NewCategoryCache(30 * time.Second)

	srv := &transport.Server{
		Auth: &transport.AuthDeps{
			Renderer:     renderer,
			Users:        userRepo,
			Sessions:     sessionStore,
			SecureCookie: cfg.SecureCookie,
			SessionTTL:   cfg.SessionTTL,
		},
		Catalog: &transport.CatalogDeps{
			Renderer:     renderer,
			Catalog:      catalogRepo,
			Cache:        categoryCache,
			ReplicaState: state,
			SecureCookie: cfg.SecureCookie,
		},
		Cart: &transport.CartDeps{
			Renderer:             renderer,
			Cart:                 cartRepo,
			ReadAfterWriteWindow: cfg.ReadAfterWriteWindow,
		},
		Order: &transport.OrderDeps{
			Renderer:             renderer,
			Orders:               orderRepo,
			ReadAfterWriteWindow: cfg.ReadAfterWriteWindow,
		},
		Report: &transport.ReportDeps{
			Renderer:     renderer,
			Reports:      reportRepo,
			ReplicaState: state,
			Timeout:      cfg.ReportTimeout,
		},
		Sessions: sessionStore,
		Users:    userRepo,
		Logger:   logger,
	}

	httpServer := srv.NewServer(cfg.Addr)
	logger.Info("primary-guard-ec listening", "addr", cfg.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}

// openDB DSN を開き、Ping で疎通確認した上で connection pool 設定を適用する
func openDB(dsn, label string, logger *slog.Logger) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	dbx.ConfigurePool(db, dbx.DefaultPoolConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		obs.CloseAndLog(db, "db ping failed")
		return nil, err
	}
	logger.Info("db opened", "label", label)
	return db, nil
}
