package dbx

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// DBRouter Primary と Replica の使い分けをアプリから隠す
// 記事 4 章で示された Reader/Writer 分離の最小実装
type DBRouter struct {
	primary *sql.DB
	replica *sql.DB
	state   *ReplicaState
}

// New DBRouter を生成する
// state を nil で渡すと内部で生成する
func New(primary, replica *sql.DB, state *ReplicaState) *DBRouter {
	if state == nil {
		state = NewReplicaState()
	}
	return &DBRouter{primary: primary, replica: replica, state: state}
}

// Writer 書き込み用に常に Primary を返す
func (r *DBRouter) Writer(_ context.Context) *sql.DB {
	return r.primary
}

// Reader 読み取り用に Primary か Replica を選んで返す
// read-after-write window が ctx に乗っている、または Replica が down のときは Primary を返す
func (r *DBRouter) Reader(ctx context.Context) *sql.DB {
	if ForcePrimary(ctx) || r.state.Down() {
		return r.primary
	}
	return r.replica
}

// ReplicaState DBRouter が参照する ReplicaState を返す
// ヘルスチェック goroutine から Trip/Reset するために公開する
func (r *DBRouter) ReplicaState() *ReplicaState {
	return r.state
}

// readAfterWriteUntilKey context へ read-after-write の有効期限を入れるためのキー型
type readAfterWriteUntilKey struct{}

// WithReadAfterWrite 注文直後などに Primary を読むよう ctx に有効期限を乗せる
// window が 0 以下のときは ctx を変更しない
func WithReadAfterWrite(ctx context.Context, window time.Duration) context.Context {
	if window <= 0 {
		return ctx
	}
	return context.WithValue(ctx, readAfterWriteUntilKey{}, time.Now().Add(window))
}

// ForcePrimary ctx に有効な read-after-write window が残っているかを返す
func ForcePrimary(ctx context.Context) bool {
	v := ctx.Value(readAfterWriteUntilKey{})
	if v == nil {
		return false
	}
	until, ok := v.(time.Time)
	if !ok {
		return false
	}
	return time.Now().Before(until)
}

// Placeholders n 個の ? を ", " 区切りで連結した文字列を返す
// IN 句のような可変長プレースホルダーを安全に作るためのヘルパ
// 値そのものは Sprintf ではなくバインドで渡す前提
func Placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}
