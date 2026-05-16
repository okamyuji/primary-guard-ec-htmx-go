package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/degrade"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
)

// ReportReader レポートに必要な操作だけを表す
type ReportReader interface {
	Monthly(ctx context.Context, year int, month time.Month) ([]domain.DailySalesSummary, error)
}

// ReportDeps レポート系ハンドラの依存
type ReportDeps struct {
	Renderer     *render.Renderer
	Reports      ReportReader
	ReplicaState *dbx.ReplicaState
	Timeout      time.Duration
	Clock        func() time.Time
}

// Monthly 当月の日次集計 (GET /admin/report)
// 重い集計を Primary に詰まらせないよう timeout を必ず付ける
func (d *ReportDeps) Monthly(w http.ResponseWriter, r *http.Request) {
	if !degrade.ReportEnabled(d.ReplicaState) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Replicaが一時停止中のためレポートを停止しています", http.StatusServiceUnavailable)
		return
	}
	clock := d.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()
	ctx, cancel := context.WithTimeout(r.Context(), d.Timeout)
	defer cancel()
	rows, err := d.Reports.Monthly(ctx, now.Year(), now.Month())
	if err != nil {
		http.Error(w, "レポートの取得に時間がかかっています。時間をおいて再度お試しください。", http.StatusServiceUnavailable)
		return
	}
	pd := NewPageData(r.Context(), "月次レポート | primary-guard-ec", true)
	data := struct {
		PageData
		Rows []domain.DailySalesSummary
	}{PageData: pd, Rows: rows}
	if err := d.Renderer.Page(w, "admin_report.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
