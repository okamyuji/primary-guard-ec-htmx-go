// Package obs ログや計測まわりの薄いユーティリティを置く
package obs

import (
	"database/sql"
	"io"
	"log/slog"
)

// CloseAndLog io.Closer を閉じてエラーがあれば label 付きで Warn ログに記録する
// defer で使われることを想定し戻り値は返さない
func CloseAndLog(c io.Closer, label string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		slog.Default().Warn("close failed", "label", label, "err", err)
	}
}

// RollbackAndLog 既に Commit 済みのときに呼ばれることがあるため
// sql.ErrTxDone は無視し、それ以外のエラーは Warn ログに記録する
func RollbackAndLog(tx *sql.Tx, label string) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		slog.Default().Warn("rollback failed", "label", label, "err", err)
	}
}

// WriteAndLog io.Writer.Write のエラーを Warn ログに記録する
// ヘルスチェック等で短いバイト列を書く用途のみを想定する
func WriteAndLog(w io.Writer, p []byte, label string) {
	if _, err := w.Write(p); err != nil {
		slog.Default().Warn("write failed", "label", label, "err", err)
	}
}
