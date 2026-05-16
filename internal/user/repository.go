// Package user ユーザー作成・ログイン認証・管理者判定を提供する
package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/auth"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"
	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/domain"
)

// ErrUserNotFound ユーザーが見つからない
var ErrUserNotFound = errors.New("user not found")

// ErrInvalidCredentials メールまたはパスワードが正しくない
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrEmailTaken メールアドレスが既に登録済み
var ErrEmailTaken = errors.New("email already taken")

// Repository ユーザーリポジトリ
// 認証は Writer 経由 (Primary) で読む
type Repository struct {
	router *dbx.DBRouter
	now    func() time.Time
}

// NewRepository 新しい Repository を生成する
func NewRepository(router *dbx.DBRouter) *Repository {
	return &Repository{router: router, now: time.Now}
}

// Register 新規ユーザーを作成して ID を返す
// メールアドレスは小文字化して保存する
func (r *Repository) Register(ctx context.Context, email, password string, isAdmin bool) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return 0, errors.New("email is empty")
	}
	if len(password) < 8 {
		return 0, errors.New("password must be at least 8 characters")
	}
	hp, err := auth.Hash(password)
	if err != nil {
		return 0, err
	}
	const q = `INSERT INTO users (email, password_hash, password_salt, password_iter, is_admin, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	admin := 0
	if isAdmin {
		admin = 1
	}
	res, err := r.router.Writer(ctx).ExecContext(ctx, q,
		email, hp.Hash, hp.Salt, hp.Iter, admin, r.now().UTC())
	if err != nil {
		if isMySQLDuplicate(err) {
			return 0, ErrEmailTaken
		}
		return 0, fmt.Errorf("register: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// Authenticate メールとパスワードで認証する
// 成功時は User を返す
func (r *Repository) Authenticate(ctx context.Context, email, password string) (domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	const q = `SELECT id, email, password_hash, password_salt, password_iter, is_admin, created_at
		FROM users WHERE email = ?`
	row := r.router.Writer(ctx).QueryRowContext(ctx, q, email)
	var (
		id          int64
		hash, salt  []byte
		iter        int
		isAdmin     int
		createdAt   time.Time
		emailLoaded string
	)
	if err := row.Scan(&id, &emailLoaded, &hash, &salt, &iter, &isAdmin, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, ErrInvalidCredentials
		}
		return domain.User{}, fmt.Errorf("authenticate scan: %w", err)
	}
	if !auth.Verify(password, auth.HashedPassword{Hash: hash, Salt: salt, Iter: iter}) {
		return domain.User{}, ErrInvalidCredentials
	}
	return domain.User{
		ID:        id,
		Email:     emailLoaded,
		IsAdmin:   isAdmin == 1,
		CreatedAt: createdAt,
	}, nil
}

// IsAdmin 指定ユーザーが管理者かどうかを返す
// auth.UserLookup インタフェースを満たす
func (r *Repository) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	const q = `SELECT is_admin FROM users WHERE id = ?`
	var v int
	err := r.router.Reader(ctx).QueryRowContext(ctx, q, userID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("is admin: %w", err)
	}
	return v == 1, nil
}

// FindByID ユーザーを 1 件取得する
func (r *Repository) FindByID(ctx context.Context, userID int64) (domain.User, error) {
	const q = `SELECT id, email, is_admin, created_at FROM users WHERE id = ?`
	var u domain.User
	var isAdmin int
	err := r.router.Reader(ctx).QueryRowContext(ctx, q, userID).Scan(
		&u.ID, &u.Email, &isAdmin, &u.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user: %w", err)
	}
	u.IsAdmin = isAdmin == 1
	return u, nil
}

// isMySQLDuplicate MySQL の duplicate entry エラーかどうかを文字列で判定する
// 標準ライブラリだけで判定するため文字列マッチを使う
func isMySQLDuplicate(err error) bool {
	return strings.Contains(err.Error(), "Error 1062") ||
		strings.Contains(err.Error(), "Duplicate entry")
}
