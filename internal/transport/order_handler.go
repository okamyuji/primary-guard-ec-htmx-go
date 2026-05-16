package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/order"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
)

// OrderManager 注文ハンドラに必要な操作だけを表す
type OrderManager interface {
	CreateOrder(ctx context.Context, userID int64) (domain.Order, error)
	FindOrder(ctx context.Context, orderID, userID int64) (domain.Order, error)
	History(ctx context.Context, userID int64, limit int) ([]domain.Order, error)
}

// OrderDeps 注文系ハンドラの依存
type OrderDeps struct {
	Renderer             *render.Renderer
	Orders               OrderManager
	ReadAfterWriteWindow time.Duration
}

// Create 注文確定 (POST /orders)
// 確定後、read-after-write window を ctx に乗せて直後の詳細を Primary で読む
func (d *OrderDeps) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	ord, err := d.Orders.CreateOrder(r.Context(), uid)
	if errors.Is(err, order.ErrEmptyCart) {
		http.Error(w, "カートが空です", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "注文に失敗しました", http.StatusInternalServerError)
		return
	}
	ctx := dbx.WithReadAfterWrite(r.Context(), d.ReadAfterWriteWindow)
	fresh, err := d.Orders.FindOrder(ctx, ord.ID, uid)
	if err != nil {
		http.Error(w, "注文の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(ctx, "注文完了 | primary-guard-ec", false)
	data := struct {
		PageData
		Order domain.Order
	}{PageData: pd, Order: fresh}
	if err := d.Renderer.Page(w, "order_complete.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Detail 注文詳細 (GET /orders/{id})
// 確定直後 (URL に "fresh=1" がついたとき) は Primary を読む
func (d *OrderDeps) Detail(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "注文IDが不正です", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if r.URL.Query().Get("fresh") == "1" {
		ctx = dbx.WithReadAfterWrite(ctx, d.ReadAfterWriteWindow)
	}
	ord, err := d.Orders.FindOrder(ctx, id, uid)
	if errors.Is(err, order.ErrOrderNotFound) {
		http.Error(w, "注文が見つかりません", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "注文詳細の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(ctx, "注文詳細 | primary-guard-ec", false)
	data := struct {
		PageData
		Order domain.Order
	}{PageData: pd, Order: ord}
	if err := d.Renderer.Page(w, "order_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// History 注文履歴 (GET /orders)
func (d *OrderDeps) History(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	orders, err := d.Orders.History(r.Context(), uid, 30)
	if err != nil {
		http.Error(w, "注文履歴の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "注文履歴 | primary-guard-ec", false)
	data := struct {
		PageData
		Orders []domain.Order
	}{PageData: pd, Orders: orders}
	if err := d.Renderer.Page(w, "order_history.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
