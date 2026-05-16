package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DBSessionStore sessions テーブルを使った SessionStore の実装
// SQL は必ずパラメータバインドで書き、Sprintf による組み立てはしない
type DBSessionStore struct {
	db  *sql.DB
	now func() time.Time
}

// NewDBSessionStore Primary 用の *sql.DB を受け取って DBSessionStore を生成する
func NewDBSessionStore(db *sql.DB) *DBSessionStore {
	return &DBSessionStore{db: db, now: time.Now}
}

// Create 新しいセッションを作って sessions テーブルに INSERT する
func (s *DBSessionStore) Create(ctx context.Context, userID int64, ttl time.Duration, csrfToken string) (Session, error) {
	id, err := NewSessionID()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	expires := now.Add(ttl)
	sess := Session{
		ID:        id,
		UserID:    userID,
		CSRFToken: csrfToken,
		ExpiresAt: expires,
		CreatedAt: now,
	}
	const q = `INSERT INTO sessions (id, user_id, csrf_token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, sess.ID, sess.UserID, sess.CSRFToken,
		sess.ExpiresAt, sess.CreatedAt); err != nil {
		return Session{}, fmt.Errorf("session insert: %w", err)
	}
	return sess, nil
}

// Find ID に紐付くセッションを返す
// 期限切れの場合は ErrSessionNotFound を返す
func (s *DBSessionStore) Find(ctx context.Context, id string) (Session, error) {
	const q = `SELECT id, user_id, csrf_token, expires_at, created_at
		FROM sessions WHERE id = ?`
	var sess Session
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&sess.ID, &sess.UserID, &sess.CSRFToken, &sess.ExpiresAt, &sess.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("session select: %w", err)
	}
	if !sess.ExpiresAt.After(s.now().UTC()) {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

// Delete セッションを 1 件削除する
func (s *DBSessionStore) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM sessions WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}

// DeleteExpired now より前に有効期限切れのセッションを削除する
// 影響行数を返す
func (s *DBSessionStore) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	const q = `DELETE FROM sessions WHERE expires_at <= ?`
	res, err := s.db.ExecContext(ctx, q, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("session delete expired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("session rows affected: %w", err)
	}
	return n, nil
}
