package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/catalog"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/degrade"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/render"
)

// CatalogReader 商品一覧と詳細とサジェストに必要な操作だけを表す
type CatalogReader interface {
	ListProducts(ctx context.Context, page, perPage int) ([]domain.Product, bool, error)
	FindProduct(ctx context.Context, id int64) (domain.Product, int, error)
	Suggest(ctx context.Context, query string, limit int) ([]domain.Product, error)
	ListCategories(ctx context.Context) ([]domain.Category, error)
}

// CategoryCacheReader CategoryCache がほしい操作だけを表す
type CategoryCacheReader interface {
	Get(ctx context.Context, repo catalog.CategoryFetcher) ([]domain.Category, error)
}

// CatalogDeps 商品系ハンドラの依存
type CatalogDeps struct {
	Renderer     *render.Renderer
	Catalog      CatalogReader
	Cache        CategoryCacheReader
	ReplicaState *dbx.ReplicaState
	SecureCookie bool
}

// Home 商品一覧 (GET /)
func (d *CatalogDeps) Home(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 20
	if degrade.Active(d.ReplicaState) {
		perPage = 6
	}

	products, hasNext, err := d.Catalog.ListProducts(r.Context(), page, perPage)
	if err != nil {
		http.Error(w, "商品一覧の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}

	pd := NewPageData(r.Context(), "商品一覧 | primary-guard-ec", false)
	pd.CSRFToken = token

	data := struct {
		PageData
		Products []domain.Product
		Page     int
		HasNext  bool
		Degraded bool
	}{
		PageData: pd,
		Products: products,
		Page:     page,
		HasNext:  hasNext,
		Degraded: degrade.Active(d.ReplicaState),
	}
	if err := d.Renderer.Page(w, "home.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Detail 商品詳細 (GET /products/{id})
func (d *CatalogDeps) Detail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "商品IDが不正です", http.StatusBadRequest)
		return
	}
	product, stock, err := d.Catalog.FindProduct(r.Context(), id)
	if errors.Is(err, catalog.ErrProductNotFound) {
		http.Error(w, "商品が見つかりません", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "商品詳細の取得に失敗しました", http.StatusInternalServerError)
		return
	}
	token, err := ensureCSRFToken(w, r, d.SecureCookie)
	if err != nil {
		http.Error(w, "CSRFトークンの発行に失敗しました", http.StatusInternalServerError)
		return
	}

	pd := NewPageData(r.Context(), product.Name+" | primary-guard-ec", false)
	pd.CSRFToken = token
	data := struct {
		PageData
		Product domain.Product
		Stock   int
	}{PageData: pd, Product: product, Stock: stock}
	if err := d.Renderer.Page(w, "product_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Suggest 検索サジェスト (GET /suggest)
// HTMX 部分更新で _suggest.html を返す
func (d *CatalogDeps) Suggest(w http.ResponseWriter, r *http.Request) {
	if !degrade.SuggestEnabled(d.ReplicaState) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query().Get("q")
	products, err := d.Catalog.Suggest(r.Context(), q, 10)
	if err != nil {
		http.Error(w, "サジェスト取得に失敗しました", http.StatusInternalServerError)
		return
	}
	if err := d.Renderer.Partial(w, "suggest.html", products); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
