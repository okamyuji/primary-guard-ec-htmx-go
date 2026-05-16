package catalog

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

type stubFetcher struct {
	calls atomic.Int32
	items []domain.Category
	delay time.Duration
}

func (s *stubFetcher) ListCategories(_ context.Context) ([]domain.Category, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.items, nil
}

// TestCacheHitWithinTTL TTL 以内ではフェッチが呼ばれないことを確認する
func TestCacheHitWithinTTL(t *testing.T) {
	t.Parallel()

	stub := &stubFetcher{items: []domain.Category{{ID: 1, Name: "a"}}}
	c := NewCategoryCache(200 * time.Millisecond)

	if _, err := c.Get(context.Background(), stub); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := c.Get(context.Background(), stub); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("fetcher calls got %d want 1", got)
	}
}

// TestCacheRefreshAfterTTL TTL 経過後はフェッチが再度呼ばれる
func TestCacheRefreshAfterTTL(t *testing.T) {
	t.Parallel()

	stub := &stubFetcher{items: []domain.Category{{ID: 1, Name: "a"}}}
	c := NewCategoryCache(20 * time.Millisecond)

	if _, err := c.Get(context.Background(), stub); err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := c.Get(context.Background(), stub); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Fatalf("calls got %d want 2", got)
	}
}

// TestCacheCoalescesParallelMiss 並行で miss しても fetcher は 1 回しか呼ばれない
func TestCacheCoalescesParallelMiss(t *testing.T) {
	t.Parallel()

	stub := &stubFetcher{
		items: []domain.Category{{ID: 1, Name: "a"}},
		delay: 30 * time.Millisecond,
	}
	c := NewCategoryCache(50 * time.Millisecond)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := c.Get(context.Background(), stub); err != nil {
				t.Errorf("get: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("calls got %d want 1 (cache stampede 抑制)", got)
	}
}

// TestCacheReturnsCopy 返り値を破壊しても次回 Get に影響しないことを確認する
func TestCacheReturnsCopy(t *testing.T) {
	t.Parallel()

	stub := &stubFetcher{items: []domain.Category{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}}
	c := NewCategoryCache(time.Hour)

	first, err := c.Get(context.Background(), stub)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	first[0].Name = "broken"

	second, err := c.Get(context.Background(), stub)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second[0].Name != "a" {
		t.Fatalf("internal mutated: %v", second[0])
	}
}
