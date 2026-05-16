// Package cart カートの追加・更新・削除・一覧を扱う
// 書き込み系は Writer 経由で実行する
package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
)

// Repository カートリポジトリ
type Repository struct {
	router *dbx.DBRouter
	now    func() time.Time
}

// NewRepository 新しい Repository を生成する
func NewRepository(router *dbx.DBRouter) *Repository {
	return &Repository{router: router, now: time.Now}
}

// ErrInvalidQuantity 数量が範囲外
var ErrInvalidQuantity = errors.New("invalid quantity")

// Add 商品をカートに追加する。すでにあれば加算する
func (r *Repository) Add(ctx context.Context, userID, productID int64, qty int) error {
	if qty <= 0 || qty > 99 {
		return ErrInvalidQuantity
	}
	const q = `INSERT INTO cart_items (user_id, product_id, quantity, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			quantity = LEAST(99, quantity + VALUES(quantity)),
			updated_at = VALUES(updated_at)`
	now := r.now().UTC()
	if _, err := r.router.Writer(ctx).ExecContext(ctx, q, userID, productID, qty, now); err != nil {
		return fmt.Errorf("cart add: %w", err)
	}
	return nil
}

// Update カート行の数量を変更する
func (r *Repository) Update(ctx context.Context, userID, productID int64, qty int) error {
	if qty <= 0 || qty > 99 {
		return ErrInvalidQuantity
	}
	const q = `UPDATE cart_items SET quantity = ?, updated_at = ?
		WHERE user_id = ? AND product_id = ?`
	res, err := r.router.Writer(ctx).ExecContext(ctx, q, qty, r.now().UTC(), userID, productID)
	if err != nil {
		return fmt.Errorf("cart update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cart update rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Remove カート行を削除する
func (r *Repository) Remove(ctx context.Context, userID, productID int64) error {
	const q = `DELETE FROM cart_items WHERE user_id = ? AND product_id = ?`
	if _, err := r.router.Writer(ctx).ExecContext(ctx, q, userID, productID); err != nil {
		return fmt.Errorf("cart remove: %w", err)
	}
	return nil
}

// List カート行と合計を返す
// 注文確定直後でも自分の最新状態を見るため Writer (Primary) を読む
func (r *Repository) List(ctx context.Context, userID int64) ([]domain.CartRow, int, error) {
	const q = `SELECT c.product_id, p.name, c.quantity, p.price_yen
		FROM cart_items c JOIN products p ON p.id = c.product_id
		WHERE c.user_id = ?
		ORDER BY c.updated_at DESC, c.product_id ASC`
	rows, err := r.router.Writer(ctx).QueryContext(ctx, q, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("cart list: %w", err)
	}
	defer obs.CloseAndLog(rows, "cart rows")
	out := make([]domain.CartRow, 0, 16)
	total := 0
	for rows.Next() {
		var row domain.CartRow
		if err := rows.Scan(&row.ProductID, &row.Name, &row.Quantity, &row.UnitPriceYen); err != nil {
			return nil, 0, fmt.Errorf("cart list scan: %w", err)
		}
		row.Subtotal = row.Quantity * row.UnitPriceYen
		total += row.Subtotal
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("cart list rows: %w", err)
	}
	return out, total, nil
}

// Clear ユーザーのカートを全削除する。注文確定の Tx で使う
// tx 経由のため Repository には外で開始した sql.Tx を渡す
func (r *Repository) Clear(ctx context.Context, tx *sql.Tx, userID int64) error {
	const q = `DELETE FROM cart_items WHERE user_id = ?`
	if _, err := tx.ExecContext(ctx, q, userID); err != nil {
		return fmt.Errorf("cart clear: %w", err)
	}
	return nil
}
