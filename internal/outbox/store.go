// Package outbox 注文確定と同じ Tx で書く outbox_events と worker による後処理を扱う
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
)

// Store outbox_events への書き込みと取り出しを行う
type Store struct {
	router *dbx.DBRouter
	now    func() time.Time
}

// NewStore 新しい Store を生成する
func NewStore(router *dbx.DBRouter) *Store {
	return &Store{router: router, now: time.Now}
}

// Insert 注文確定など書き込み Tx 内で同時に呼ぶ
// 呼び出し側の Tx を渡す
func (s *Store) Insert(ctx context.Context, tx *sql.Tx, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox marshal: %w", err)
	}
	const q = `INSERT INTO outbox_events (event_type, payload, processed_at, created_at)
		VALUES (?, ?, NULL, ?)`
	if _, err := tx.ExecContext(ctx, q, eventType, raw, s.now().UTC()); err != nil {
		return fmt.Errorf("outbox insert: %w", err)
	}
	return nil
}

// Pending 未処理イベントを最大 limit 件取り出す
// 同時走行する worker でも安全にするため FOR UPDATE SKIP LOCKED で取る
// 呼び出し側は別の Tx を開始してこの関数を呼び、続けて MarkProcessed を呼ぶ
func (s *Store) Pending(ctx context.Context, tx *sql.Tx, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT id, event_type, payload, created_at
		FROM outbox_events
		WHERE processed_at IS NULL
		ORDER BY id ASC
		LIMIT ?
		FOR UPDATE SKIP LOCKED`
	rows, err := tx.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox pending: %w", err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		var raw []byte
		if err := rows.Scan(&e.ID, &e.Type, &raw, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("outbox scan: %w", err)
		}
		e.Payload = json.RawMessage(raw)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox rows: %w", err)
	}
	return events, nil
}

// MarkProcessed 指定 id を処理済みにする
func (s *Store) MarkProcessed(ctx context.Context, tx *sql.Tx, id int64) error {
	const q = `UPDATE outbox_events SET processed_at = ? WHERE id = ?`
	if _, err := tx.ExecContext(ctx, q, s.now().UTC(), id); err != nil {
		return fmt.Errorf("outbox mark processed: %w", err)
	}
	return nil
}

// Event worker が処理するイベント
type Event struct {
	ID        int64
	Type      string
	Payload   json.RawMessage
	CreatedAt time.Time
}

// Begin Primary へのトランザクションを開始する
// worker が outbox を取りに行く際に使う
func (s *Store) Begin(ctx context.Context) (*sql.Tx, error) {
	return s.router.Writer(ctx).BeginTx(ctx, nil)
}
