// Package report 管理画面向けの集計レポートを Reader と集計テーブルから読み出す
package report

import (
	"context"
	"fmt"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

// Repository レポートリポジトリ
type Repository struct {
	router *dbx.DBRouter
}

// NewRepository 新しい Repository を生成する
func NewRepository(router *dbx.DBRouter) *Repository {
	return &Repository{router: router}
}

// Monthly 指定月の日次集計を返す
// daily_sales_summary を Reader から SELECT する
func (r *Repository) Monthly(ctx context.Context, year int, month time.Month) ([]domain.DailySalesSummary, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	const q = `SELECT sales_date, total_orders, total_amount, updated_at
		FROM daily_sales_summary
		WHERE sales_date >= ? AND sales_date < ?
		ORDER BY sales_date ASC`
	rows, err := r.router.Reader(ctx).QueryContext(ctx, q, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("monthly: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]domain.DailySalesSummary, 0, 31)
	for rows.Next() {
		var s domain.DailySalesSummary
		if err := rows.Scan(&s.Date, &s.TotalOrders, &s.TotalAmount, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("monthly scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("monthly rows: %w", err)
	}
	return out, nil
}
