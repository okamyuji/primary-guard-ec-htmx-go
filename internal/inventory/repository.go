// Package inventory 在庫の引当処理を扱う
// すべての書き込みは呼び出し側が開始した Tx 内で実行する
package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrInsufficientStock 在庫不足
var ErrInsufficientStock = errors.New("insufficient stock")

// Repository 在庫リポジトリ
type Repository struct {
	now func() time.Time
}

// NewRepository 新しい Repository を生成する
func NewRepository() *Repository {
	return &Repository{now: time.Now}
}

// Reserve 在庫を qty だけ減らす
// 在庫不足なら ErrInsufficientStock を返す
// 呼び出し側が開始した *sql.Tx を渡す
func (r *Repository) Reserve(ctx context.Context, tx *sql.Tx, productID int64, qty int) error {
	if qty <= 0 {
		return errors.New("inventory reserve: qty must be positive")
	}
	const q = `UPDATE inventory SET stock = stock - ?, updated_at = ?
		WHERE product_id = ? AND stock >= ?`
	res, err := tx.ExecContext(ctx, q, qty, r.now().UTC(), productID, qty)
	if err != nil {
		return fmt.Errorf("inventory reserve: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("inventory reserve rows: %w", err)
	}
	if n == 0 {
		return ErrInsufficientStock
	}
	return nil
}
