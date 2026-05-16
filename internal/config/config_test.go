package config

import (
	"testing"
	"time"
)

// TestLoadRequiresDSN Primary 未設定でエラーになることを確認する
func TestLoadRequiresDSN(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "")
	t.Setenv("DB_REPLICA_DSN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("err want non-nil")
	}
}

// TestLoadReplicaFallsBackToPrimary Replica 未設定時は Primary が流用される
func TestLoadReplicaFallsBackToPrimary(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "primary-only")
	t.Setenv("DB_REPLICA_DSN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ReplicaDSN != "primary-only" {
		t.Fatalf("replica got %s want primary-only", cfg.ReplicaDSN)
	}
}

// TestLoadAppliesDefaults 既定値が入ることを確認する
func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "primary")
	t.Setenv("DB_REPLICA_DSN", "replica")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr got %s want :8080", cfg.Addr)
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Errorf("SessionTTL got %s want 24h", cfg.SessionTTL)
	}
	if cfg.ReadAfterWriteWindow != 5*time.Second {
		t.Errorf("ReadAfterWriteWindow got %s want 5s", cfg.ReadAfterWriteWindow)
	}
	if cfg.OutboxBatch != 50 {
		t.Errorf("OutboxBatch got %d want 50", cfg.OutboxBatch)
	}
	if cfg.SecureCookie {
		t.Errorf("SecureCookie default want false")
	}
	if cfg.MigrationsDir != "migrations" {
		t.Errorf("MigrationsDir got %s want migrations", cfg.MigrationsDir)
	}
}

// TestLoadOverridesFromEnv 値を上書きできることを確認する
func TestLoadOverridesFromEnv(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "primary")
	t.Setenv("DB_REPLICA_DSN", "replica")
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_SECURE_COOKIE", "true")
	t.Setenv("APP_SESSION_TTL", "1h")
	t.Setenv("APP_OUTBOX_BATCH", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr got %s want :9090", cfg.Addr)
	}
	if !cfg.SecureCookie {
		t.Errorf("SecureCookie want true")
	}
	if cfg.SessionTTL != time.Hour {
		t.Errorf("SessionTTL got %s want 1h", cfg.SessionTTL)
	}
	if cfg.OutboxBatch != 10 {
		t.Errorf("OutboxBatch got %d want 10", cfg.OutboxBatch)
	}
}

// TestLoadRejectsBadBool 不正な真偽値でエラーになることを確認する
func TestLoadRejectsBadBool(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "primary")
	t.Setenv("DB_REPLICA_DSN", "replica")
	t.Setenv("APP_SECURE_COOKIE", "noyes")

	if _, err := Load(); err == nil {
		t.Fatal("err want non-nil")
	}
}

// TestLoadRejectsZeroDuration ゼロや負の duration を拒否する
func TestLoadRejectsZeroDuration(t *testing.T) {
	t.Setenv("DB_PRIMARY_DSN", "primary")
	t.Setenv("DB_REPLICA_DSN", "replica")
	t.Setenv("APP_SESSION_TTL", "0s")

	if _, err := Load(); err == nil {
		t.Fatal("err want non-nil")
	}
}
