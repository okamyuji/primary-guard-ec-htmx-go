// Package degrade Replica 障害時に縮退運転を発動するかを判定する小さなユーティリティ
package degrade

import "github.com/okamyuji/primary-guard-ec-htmx-go/internal/dbx"

// Active Replica が down と判断されているか
func Active(state *dbx.ReplicaState) bool {
	if state == nil {
		return false
	}
	return state.Down()
}

// ReportEnabled 管理画面レポートを通常表示してよいか
// Replica が down のときは false にして Primary への負荷集中を防ぐ
func ReportEnabled(state *dbx.ReplicaState) bool {
	return !Active(state)
}

// SuggestEnabled サジェストを返してよいか
// Replica が down のときは false にして高頻度の軽い read を止める
func SuggestEnabled(state *dbx.ReplicaState) bool {
	return !Active(state)
}
