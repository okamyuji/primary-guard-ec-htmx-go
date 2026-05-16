package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
)

// Handler 1 件のイベントを処理する関数型
// メール送信や検索インデックス更新などのスタブをここに差し込む
type Handler func(ctx context.Context, ev Event) error

// Worker 未処理イベントをポーリングして処理する
type Worker struct {
	store    *Store
	handler  Handler
	interval time.Duration
	batch    int
	logger   *slog.Logger
}

// NewWorker 新しい Worker を生成する
// logger が nil なら slog.Default を使う
func NewWorker(store *Store, handler Handler, interval time.Duration, batch int, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 50
	}
	return &Worker{
		store:    store,
		handler:  handler,
		interval: interval,
		batch:    batch,
		logger:   logger,
	}
}

// Run ctx がキャンセルされるまで定期的にポーリングしてイベントを処理する
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
			return
		case <-t.C:
			if err := w.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				w.logger.Warn("outbox tick failed", "err", err)
			}
		}
	}
}

// tick 1 回のポーリングと処理を行う
func (w *Worker) tick(ctx context.Context) error {
	tx, err := w.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			obs.RollbackAndLog(tx, "outbox worker tick")
		}
	}()

	events, err := w.store.Pending(ctx, tx, w.batch)
	if err != nil {
		return fmt.Errorf("pending: %w", err)
	}
	if len(events) == 0 {
		committed = true
		return tx.Commit()
	}
	for _, ev := range events {
		if err := w.handler(ctx, ev); err != nil {
			w.logger.Warn("outbox handler failed", "id", ev.ID, "type", ev.Type, "err", err)
			continue
		}
		if err := w.store.MarkProcessed(ctx, tx, ev.ID); err != nil {
			return fmt.Errorf("mark processed: %w", err)
		}
	}
	committed = true
	return tx.Commit()
}

// LogHandler 受け取ったイベントを INFO ログに出すだけのデフォルトハンドラ
func LogHandler(logger *slog.Logger) Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(_ context.Context, ev Event) error {
		logger.Info("outbox event handled",
			"id", ev.ID,
			"type", ev.Type,
			"created_at", ev.CreatedAt)
		return nil
	}
}
