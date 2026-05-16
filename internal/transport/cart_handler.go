package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/cart"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
)

// CartManager カート操作に必要な操作だけを表す
type CartManager interface {
	Add(ctx context.Context, userID, productID int64, qty int) error
	Update(ctx context.Context, userID, productID int64, qty int) error
	Remove(ctx context.Context, userID, productID int64) error
	List(ctx context.Context, userID int64) ([]domain.CartRow, int, error)
}

// CartDeps カート系ハンドラの依存
type CartDeps struct {
	Renderer             *render.Renderer
	Cart                 CartManager
	ReadAfterWriteWindow time.Duration
}

// View カート画面 (GET /cart)
// 自分のカートを正しく見るため Writer 側 Repository が Primary を読む
func (d *CartDeps) View(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	rows, total, err := d.Cart.List(r.Context(), uid)
	if err != nil {
		http.Error(w, "カートの取得に失敗しました", http.StatusInternalServerError)
		return
	}

	pd := NewPageData(r.Context(), "カート | primary-guard-ec", false)
	data := struct {
		PageData
		Rows  []cartViewRow
		Total int
	}{PageData: pd, Rows: toViewRows(rows, pd.CSRFToken), Total: total}
	if err := d.Renderer.Page(w, "cart.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Add カート追加 (POST /cart/add)
// HTMX 経由の場合でも通常 form 経由でもどちらでも動くよう、Redirect で /cart に戻す
func (d *CartDeps) Add(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	productID, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	if err != nil {
		http.Error(w, "商品IDが不正です", http.StatusBadRequest)
		return
	}
	qty, err := strconv.Atoi(r.PostFormValue("quantity"))
	if err != nil {
		http.Error(w, "数量が不正です", http.StatusBadRequest)
		return
	}
	if err := d.Cart.Add(r.Context(), uid, productID, qty); err != nil {
		if errors.Is(err, cart.ErrInvalidQuantity) {
			http.Error(w, "数量が範囲外です", http.StatusBadRequest)
			return
		}
		http.Error(w, "カートへの追加に失敗しました", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

// Update カート数量変更 (POST /cart/update)
// HTMX で行単位差し替えを想定し、_cart_row.html を返す
func (d *CartDeps) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Error(w, "ログインが必要です", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	productID, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	if err != nil {
		http.Error(w, "商品IDが不正です", http.StatusBadRequest)
		return
	}
	qty, err := strconv.Atoi(r.PostFormValue("quantity"))
	if err != nil {
		http.Error(w, "数量が不正です", http.StatusBadRequest)
		return
	}
	if err := d.Cart.Update(r.Context(), uid, productID, qty); err != nil {
		http.Error(w, "カートの更新に失敗しました", http.StatusInternalServerError)
		return
	}
	d.renderRow(w, r, uid, productID)
}

// Remove カート削除 (POST /cart/remove)
// HTMX で行を消すため空文字を返す
func (d *CartDeps) Remove(w http.ResponseWriter, r *http.Request) {
	uid, ok := auth.CurrentUserID(r.Context())
	if !ok {
		http.Error(w, "ログインが必要です", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "フォーム解析に失敗しました", http.StatusBadRequest)
		return
	}
	productID, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	if err != nil {
		http.Error(w, "商品IDが不正です", http.StatusBadRequest)
		return
	}
	if err := d.Cart.Remove(r.Context(), uid, productID); err != nil {
		http.Error(w, "カートからの削除に失敗しました", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// renderRow 更新後の行をテンプレートで返すヘルパ
func (d *CartDeps) renderRow(w http.ResponseWriter, r *http.Request, uid, productID int64) {
	// 行 1 件のため List をそのまま再取得し、対象だけ render する
	rows, _, err := d.Cart.List(r.Context(), uid)
	if err != nil {
		http.Error(w, "カート取得に失敗しました", http.StatusInternalServerError)
		return
	}
	pd := NewPageData(r.Context(), "", false)
	for _, row := range rows {
		if row.ProductID == productID {
			vr := toViewRow(row, pd.CSRFToken)
			if err := d.Renderer.Partial(w, "cart_row.html", vr); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// dbxAlias dbx 参照を保つためのエイリアス(未使用 import 抑止用)
var _ = dbx.ForcePrimary

// cartViewRow テンプレートに渡すカート行
type cartViewRow struct {
	ProductID    int64
	Name         string
	Quantity     int
	UnitPriceYen int
	Subtotal     int
	CSRFToken    string
}

// toViewRows CartRow をテンプレ用に変換する
func toViewRows(rows []domain.CartRow, token string) []cartViewRow {
	out := make([]cartViewRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, toViewRow(r, token))
	}
	return out
}

// toViewRow 1 行分の変換
func toViewRow(r domain.CartRow, token string) cartViewRow {
	return cartViewRow{
		ProductID:    r.ProductID,
		Name:         r.Name,
		Quantity:     r.Quantity,
		UnitPriceYen: r.UnitPriceYen,
		Subtotal:     r.Subtotal,
		CSRFToken:    token,
	}
}
