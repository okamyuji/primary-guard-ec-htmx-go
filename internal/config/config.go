// Package config 環境変数からアプリ設定を読み取り型付き Config 値として返す
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config アプリ全体で参照する設定値
type Config struct {
	Addr                  string
	SecureCookie          bool
	SessionTTL            time.Duration
	ReportTimeout         time.Duration
	ReadAfterWriteWindow  time.Duration
	ReplicaLagTripAfter   time.Duration
	ReplicaHealthInterval time.Duration
	OutboxInterval        time.Duration
	OutboxBatch           int
	PrimaryDSN            string
	ReplicaDSN            string
	MigrationsDir         string
}

// Load 環境変数から Config を構築する
// 必須項目が空または不正値のときはエラーを返す
func Load() (Config, error) {
	cfg := Config{
		Addr:          envDefault("APP_ADDR", ":8080"),
		MigrationsDir: envDefault("APP_MIGRATIONS_DIR", "migrations"),
		PrimaryDSN:    os.Getenv("DB_PRIMARY_DSN"),
		ReplicaDSN:    os.Getenv("DB_REPLICA_DSN"),
	}

	secure, err := envBool("APP_SECURE_COOKIE", false)
	if err != nil {
		return Config{}, err
	}
	cfg.SecureCookie = secure

	durations := []struct {
		key      string
		fallback time.Duration
		target   *time.Duration
	}{
		{"APP_SESSION_TTL", 24 * time.Hour, &cfg.SessionTTL},
		{"APP_REPORT_TIMEOUT", 3 * time.Second, &cfg.ReportTimeout},
		{"APP_READ_AFTER_WRITE", 5 * time.Second, &cfg.ReadAfterWriteWindow},
		{"APP_REPLICA_LAG_TRIP", 2 * time.Second, &cfg.ReplicaLagTripAfter},
		{"APP_REPLICA_HEALTH_INTERVAL", 5 * time.Second, &cfg.ReplicaHealthInterval},
		{"APP_OUTBOX_INTERVAL", 1 * time.Second, &cfg.OutboxInterval},
	}
	for _, d := range durations {
		v, err := envDuration(d.key, d.fallback)
		if err != nil {
			return Config{}, err
		}
		*d.target = v
	}

	batch, err := envInt("APP_OUTBOX_BATCH", 50)
	if err != nil {
		return Config{}, err
	}
	if batch <= 0 {
		return Config{}, fmt.Errorf("APP_OUTBOX_BATCH must be positive, got %d", batch)
	}
	cfg.OutboxBatch = batch

	if cfg.PrimaryDSN == "" {
		return Config{}, errors.New("DB_PRIMARY_DSN is required")
	}
	if cfg.ReplicaDSN == "" {
		return Config{}, errors.New("DB_REPLICA_DSN is required")
	}
	return cfg, nil
}

// envDefault 未設定や空文字なら fallback を返す
func envDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

// envBool 環境変数を真偽値として読む
func envBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s parse bool: %w", key, err)
	}
	return b, nil
}

// envDuration 環境変数を time.Duration として読む
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s parse duration: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive duration, got %s", key, v)
	}
	return d, nil
}

// envInt 環境変数を int として読む
func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s parse int: %w", key, err)
	}
	return n, nil
}
