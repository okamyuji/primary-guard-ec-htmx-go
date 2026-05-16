// Package main 開発・教材用にテストユーザー (管理者 + 一般ユーザー 2 名) を Primary に投入する
// 既存ユーザーがある場合はスキップする
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/config"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/user"
)

// main エントリポイント
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

// run シードユーザーを投入する
func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := sql.Open("mysql", cfg.PrimaryDSN)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	router := dbx.New(db, db, nil)
	repo := user.NewRepository(router)

	seedUsers := []struct {
		Email    string
		Password string
		IsAdmin  bool
	}{
		{"admin@example.com", "Admin12345", true},
		{"alice@example.com", "Alice12345", false},
		{"bob@example.com", "Bob1234567", false},
	}

	for _, s := range seedUsers {
		id, err := repo.Register(ctx, s.Email, s.Password, s.IsAdmin)
		if err == nil {
			logger.Info("seeded user", "email", s.Email, "id", id, "admin", s.IsAdmin)
			continue
		}
		if errors.Is(err, user.ErrEmailTaken) {
			logger.Info("seed user exists", "email", s.Email)
			continue
		}
		return fmt.Errorf("seed user %s: %w", s.Email, err)
	}
	return nil
}
