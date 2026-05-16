// Package catalog 商品とカテゴリと検索サジェストを Reader 経由で読み出す
package catalog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

// Repository 商品リポジトリ
type Repository struct {
	router *dbx.DBRouter
}

// NewRepository 新しい Repository を生成する
func NewRepository(router *dbx.DBRouter) *Repository {
	return &Repository{router: router}
}

// ListProducts ステータスが active な商品を新着順で返す
// SQL はすべてパラメータバインドで書く
func (r *Repository) ListProducts(ctx context.Context, page, perPage int) ([]domain.Product, bool, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	const q = `SELECT id, category_id, name, price_yen, status, updated_at
		FROM products
		WHERE status = ?
		ORDER BY updated_at DESC, id DESC
		LIMIT ? OFFSET ?`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, q, "active", perPage+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	products := make([]domain.Product, 0, perPage+1)
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.PriceYen, &p.Status, &p.UpdatedAt); err != nil {
			return nil, false, fmt.Errorf("list products scan: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list products rows: %w", err)
	}
	hasNext := len(products) > perPage
	if hasNext {
		products = products[:perPage]
	}
	return products, hasNext, nil
}

// FindProduct 商品 ID で 1 件取得する
// 在庫数も併せて返す
func (r *Repository) FindProduct(ctx context.Context, id int64) (domain.Product, int, error) {
	const q = `SELECT p.id, p.category_id, p.name, p.price_yen, p.status, p.updated_at,
		COALESCE(i.stock, 0)
		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.id = ?`
	var p domain.Product
	var stock int
	err := r.router.Reader(ctx).QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.CategoryID, &p.Name, &p.PriceYen, &p.Status, &p.UpdatedAt, &stock,
	)
	if err == sql.ErrNoRows {
		return domain.Product{}, 0, ErrProductNotFound
	}
	if err != nil {
		return domain.Product{}, 0, fmt.Errorf("find product: %w", err)
	}
	return p, stock, nil
}

// Suggest 部分一致で商品名を最大 limit 件返す
func (r *Repository) Suggest(ctx context.Context, query string, limit int) ([]domain.Product, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	const q = `SELECT id, category_id, name, price_yen, status, updated_at
		FROM products
		WHERE status = ? AND name LIKE ?
		ORDER BY name ASC
		LIMIT ?`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, q, "active", "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("suggest: %w", err)
	}
	defer func() { _ = rows.Close() }()

	products := make([]domain.Product, 0, limit)
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.Name, &p.PriceYen, &p.Status, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("suggest scan: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("suggest rows: %w", err)
	}
	return products, nil
}

// ListCategories 全カテゴリを名前順で返す
func (r *Repository) ListCategories(ctx context.Context) ([]domain.Category, error) {
	const q = `SELECT id, name, updated_at FROM categories ORDER BY name ASC`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	categories := make([]domain.Category, 0, 32)
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list categories scan: %w", err)
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list categories rows: %w", err)
	}
	return categories, nil
}
