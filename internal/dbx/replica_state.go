package dbx

import "sync/atomic"

// ReplicaState Replica が利用可能かどうかを atomic に保持する
// 複数 goroutine からの読み書きを安全に行う
type ReplicaState struct {
	down atomic.Bool
}

// NewReplicaState 新しい ReplicaState を生成する
// 初期状態は up とみなす
func NewReplicaState() *ReplicaState {
	return &ReplicaState{}
}

// Trip Replica を down として記録する
func (s *ReplicaState) Trip() { s.down.Store(true) }

// Reset Replica を up として記録する
func (s *ReplicaState) Reset() { s.down.Store(false) }

// Down 現在の状態を返す
func (s *ReplicaState) Down() bool { return s.down.Load() }
