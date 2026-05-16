package catalog

import (
	"context"
	"sync"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

// CategoryCache カテゴリ一覧を短時間だけメモリに保持する
// 記事 5 章の CategoryCache をそのまま実装し cache stampede を抑制する
type CategoryCache struct {
	mu        sync.RWMutex
	items     []domain.Category
	expiresAt time.Time
	ttl       time.Duration
}

// NewCategoryCache 指定 TTL の CategoryCache を生成する
func NewCategoryCache(ttl time.Duration) *CategoryCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &CategoryCache{ttl: ttl}
}

// CategoryFetcher cache が miss した時に呼ばれる取得関数
type CategoryFetcher interface {
	ListCategories(ctx context.Context) ([]domain.Category, error)
}

// Get TTL 内ならキャッシュ値、満了ならフェッチして詰め直して返す
// 戻り値は内部スライスのコピーで、呼び出し側が破壊しても安全
func (c *CategoryCache) Get(ctx context.Context, repo CategoryFetcher) ([]domain.Category, error) {
	now := time.Now()

	c.mu.RLock()
	if now.Before(c.expiresAt) {
		items := append([]domain.Category(nil), c.items...)
		c.mu.RUnlock()
		return items, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if now.Before(c.expiresAt) {
		return append([]domain.Category(nil), c.items...), nil
	}

	items, err := repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	c.items = append([]domain.Category(nil), items...)
	c.expiresAt = now.Add(c.ttl)
	return append([]domain.Category(nil), items...), nil
}
