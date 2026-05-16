// Package dbx 接続プール設定と Reader/Writer ルーティング、Replica 障害フラグを扱う
package dbx

import (
	"database/sql"
	"time"
)

// PoolConfig connection pool に渡す値
type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultPoolConfig 中小規模 Web に向けた推奨初期値を返す
// 記事 4 章で示された数字をそのまま採用する
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxOpenConns:    50,
		MaxIdleConns:    25,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// ConfigurePool 推奨値を *sql.DB に適用する
func ConfigurePool(db *sql.DB, cfg PoolConfig) {
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
}
