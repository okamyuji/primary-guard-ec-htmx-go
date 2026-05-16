// Package order 注文作成・注文詳細・注文履歴を扱う
// 書き込みは Primary、読み取りは forcePrimary 中なら Primary、それ以外は Replica
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

// ErrOrderNotFound 注文が見つからない
var ErrOrderNotFound = errors.New("order not found")

// ErrEmptyCart カートが空のときに注文しようとした
var ErrEmptyCart = errors.New("cart is empty")

// CartReader 注文作成時にカートを読み出す側に必要な操作だけを表す
type CartReader interface {
	List(ctx context.Context, userID int64) ([]domain.CartRow, int, error)
	Clear(ctx context.Context, tx *sql.Tx, userID int64) error
}

// InventoryReserver 在庫を引当する側に必要な操作だけを表す
type InventoryReserver interface {
	Reserve(ctx context.Context, tx *sql.Tx, productID int64, qty int) error
}

// OutboxWriter outbox にイベントを書く側に必要な操作だけを表す
type OutboxWriter interface {
	Insert(ctx context.Context, tx *sql.Tx, eventType string, payload any) error
}

// Repository 注文リポジトリ
// 依存は内部で必要な小さい interface だけを受ける
type Repository struct {
	router    *dbx.DBRouter
	cart      CartReader
	inventory InventoryReserver
	outbox    OutboxWriter
	now       func() time.Time
}

// NewRepository 新しい Repository を生成する
func NewRepository(router *dbx.DBRouter, cart CartReader, inv InventoryReserver, ob OutboxWriter) *Repository {
	return &Repository{
		router:    router,
		cart:      cart,
		inventory: inv,
		outbox:    ob,
		now:       time.Now,
	}
}

// CreateOrder 注文を作成する
// orders / order_items / inventory / outbox_events / daily_sales_summary を同じ Tx で書く
func (r *Repository) CreateOrder(ctx context.Context, userID int64) (domain.Order, error) {
	rows, total, err := r.cart.List(ctx, userID)
	if err != nil {
		return domain.Order{}, err
	}
	if len(rows) == 0 {
		return domain.Order{}, ErrEmptyCart
	}

	tx, err := r.router.Writer(ctx).BeginTx(ctx, nil)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := r.now().UTC()

	const insertOrder = `INSERT INTO orders (user_id, total_yen, status, created_at)
		VALUES (?, ?, ?, ?)`
	res, err := tx.ExecContext(ctx, insertOrder, userID, total, "confirmed", now)
	if err != nil {
		return domain.Order{}, fmt.Errorf("insert order: %w", err)
	}
	orderID, err := res.LastInsertId()
	if err != nil {
		return domain.Order{}, fmt.Errorf("last insert id: %w", err)
	}

	if err := insertOrderItems(ctx, tx, orderID, rows); err != nil {
		return domain.Order{}, err
	}
	for _, row := range rows {
		if err := r.inventory.Reserve(ctx, tx, row.ProductID, row.Quantity); err != nil {
			return domain.Order{}, err
		}
	}
	if err := r.cart.Clear(ctx, tx, userID); err != nil {
		return domain.Order{}, err
	}
	payload := map[string]any{
		"order_id":  orderID,
		"user_id":   userID,
		"total_yen": total,
	}
	if err := r.outbox.Insert(ctx, tx, "order_confirmed", payload); err != nil {
		return domain.Order{}, err
	}
	if err := upsertDailySummary(ctx, tx, now, total); err != nil {
		return domain.Order{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Order{}, fmt.Errorf("commit: %w", err)
	}
	committed = true

	items := make([]domain.OrderItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.OrderItem{
			OrderID:      orderID,
			ProductID:    row.ProductID,
			ProductName:  row.Name,
			Quantity:     row.Quantity,
			UnitPriceYen: row.UnitPriceYen,
		})
	}
	return domain.Order{
		ID:        orderID,
		UserID:    userID,
		TotalYen:  total,
		Status:    "confirmed",
		CreatedAt: now,
		Items:     items,
	}, nil
}

// insertOrderItems まとめて INSERT する
// SQL は ? のプレースホルダを動的に組み立てるが、値は必ずバインドで渡す
func insertOrderItems(ctx context.Context, tx *sql.Tx, orderID int64, rows []domain.CartRow) error {
	if len(rows) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*4)
	for _, row := range rows {
		placeholders = append(placeholders, "(?, ?, ?, ?)")
		args = append(args, orderID, row.ProductID, row.Quantity, row.UnitPriceYen)
	}
	//nolint:gosec // プレースホルダー (?) だけを連結しており、値は ExecContext の引数として安全に渡している
	q := "INSERT INTO order_items (order_id, product_id, quantity, unit_price_yen) VALUES " +
		strings.Join(placeholders, ", ")
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert order items: %w", err)
	}
	return nil
}

// upsertDailySummary 日次集計を加算する
// 同じ日に複数注文があれば加算され、日が変われば新規行になる
func upsertDailySummary(ctx context.Context, tx *sql.Tx, now time.Time, total int) error {
	const q = `INSERT INTO daily_sales_summary (sales_date, total_orders, total_amount, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			total_orders = total_orders + 1,
			total_amount = total_amount + VALUES(total_amount),
			updated_at = VALUES(updated_at)`
	date := now.Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, q, date, 1, total, now); err != nil {
		return fmt.Errorf("upsert daily summary: %w", err)
	}
	return nil
}

// FindOrder 注文を 1 件取得する
// forcePrimary 中は Primary を読む
func (r *Repository) FindOrder(ctx context.Context, orderID, userID int64) (domain.Order, error) {
	const headQ = `SELECT id, user_id, total_yen, status, created_at
		FROM orders WHERE id = ? AND user_id = ?`
	var ord domain.Order
	err := r.router.Reader(ctx).QueryRowContext(ctx, headQ, orderID, userID).Scan(
		&ord.ID, &ord.UserID, &ord.TotalYen, &ord.Status, &ord.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("find order: %w", err)
	}

	const itemQ = `SELECT oi.product_id, p.name, oi.quantity, oi.unit_price_yen
		FROM order_items oi JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id = ?
		ORDER BY oi.product_id ASC`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, itemQ, orderID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("find order items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var it domain.OrderItem
		if err := rows.Scan(&it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPriceYen); err != nil {
			return domain.Order{}, fmt.Errorf("scan order item: %w", err)
		}
		it.OrderID = orderID
		ord.Items = append(ord.Items, it)
	}
	if err := rows.Err(); err != nil {
		return domain.Order{}, fmt.Errorf("order items rows: %w", err)
	}
	return ord, nil
}

// History ユーザーの注文履歴を新着順で返す
func (r *Repository) History(ctx context.Context, userID int64, limit int) ([]domain.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	const q = `SELECT id, user_id, total_yen, status, created_at
		FROM orders WHERE user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.Order, 0, limit)
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.TotalYen, &o.Status, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("history scan: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history rows: %w", err)
	}
	return out, nil
}
