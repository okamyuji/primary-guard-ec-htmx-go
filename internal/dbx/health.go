package dbx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/okamyuji/primary-guard-ec-htmx-go/internal/obs"
)

// ReplicaHealth Replica の遅延を定期的に確認し、しきい値超過で ReplicaState を Trip する
// 単発の計測エラーや一時的な遅延でフラップしないよう、連続失敗回数で Trip 判定をする
type ReplicaHealth struct {
	replica       *sql.DB
	state         *ReplicaState
	interval      time.Duration
	tripAt        time.Duration
	tripAfterN    int
	logger        *slog.Logger
	now           func() time.Time
	consecutiveNG int
}

// NewReplicaHealth 新しいヘルスチェッカを生成する
// logger が nil なら slog.Default を使う
// tripAfterN は連続して NG が続いた回数で Trip するしきい値 (1 以上)
func NewReplicaHealth(replica *sql.DB, state *ReplicaState, interval, tripAt time.Duration, logger *slog.Logger) *ReplicaHealth {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if tripAt <= 0 {
		tripAt = 2 * time.Second
	}
	return &ReplicaHealth{
		replica:    replica,
		state:      state,
		interval:   interval,
		tripAt:     tripAt,
		tripAfterN: 3,
		logger:     logger,
		now:        time.Now,
	}
}

// Run ctx がキャンセルされるまで定期的に SHOW REPLICA STATUS を読む
// 遅延がしきい値を超えたら Trip、戻ったら Reset する
// 取得失敗が連続したときも Trip し、回復したら Reset する
func (h *ReplicaHealth) Run(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.evaluate(ctx)
		}
	}
}

// evaluate 1 回のチェックを行う
// 連続失敗が tripAfterN を超えるまでは Trip しない (フラップ抑制)
// 1 回でも成功すれば即 Reset し連続カウンタを 0 に戻す
func (h *ReplicaHealth) evaluate(ctx context.Context) {
	lag, err := h.checkLag(ctx)
	if err != nil {
		h.consecutiveNG++
		h.logger.Warn("replica lag check failed", "err", err, "consecutive", h.consecutiveNG)
		if h.consecutiveNG >= h.tripAfterN {
			if !h.state.Down() {
				h.logger.Warn("replica tripped by consecutive check failures", "n", h.consecutiveNG)
			}
			h.state.Trip()
		}
		return
	}
	if lag > h.tripAt {
		h.consecutiveNG++
		if h.consecutiveNG >= h.tripAfterN {
			if !h.state.Down() {
				h.logger.Warn("replica tripped by lag", "lag", lag.String(), "threshold", h.tripAt.String())
			}
			h.state.Trip()
		}
		return
	}
	if h.state.Down() {
		h.logger.Info("replica recovered", "lag", lag.String())
	}
	h.consecutiveNG = 0
	h.state.Reset()
}

// checkLag SHOW REPLICA STATUS を読んで Seconds_Behind_Source を time.Duration として返す
// Replica が動いていない場合は err になり呼び出し側で Trip する
func (h *ReplicaHealth) checkLag(ctx context.Context) (time.Duration, error) {
	rows, err := h.replica.QueryContext(ctx, "SHOW REPLICA STATUS")
	if err != nil {
		return 0, fmt.Errorf("show replica status: %w", err)
	}
	defer obs.CloseAndLog(rows, "replica status rows")

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("columns: %w", err)
	}
	if !rows.Next() {
		return 0, errors.New("replica status empty")
	}

	values := make([]any, len(cols))
	holders := make([]any, len(cols))
	for i := range values {
		holders[i] = &values[i]
	}
	if err := rows.Scan(holders...); err != nil {
		return 0, fmt.Errorf("scan: %w", err)
	}

	lookup := func(name string) any {
		for i, c := range cols {
			if c == name {
				return values[i]
			}
		}
		return nil
	}

	ioRunning := asString(lookup("Replica_IO_Running"))
	sqlRunning := asString(lookup("Replica_SQL_Running"))
	if ioRunning != "Yes" || sqlRunning != "Yes" {
		return 0, fmt.Errorf("replica threads not running io=%s sql=%s", ioRunning, sqlRunning)
	}
	// Seconds_Behind_Source は Replica が追いついて Source の更新を待っている間 NULL になる
	// IO と SQL が Yes でここまで来たので NULL は lag 0 とみなして安全に通す
	secs, ok := asInt64(lookup("Seconds_Behind_Source"))
	if !ok {
		return 0, nil
	}
	return time.Duration(secs) * time.Second, nil
}

// asString interface 値を string にする
func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}

// asInt64 interface 値を int64 にする
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case []byte:
		s := string(n)
		var out int64
		for _, c := range s {
			if c < '0' || c > '9' {
				return 0, false
			}
			out = out*10 + int64(c-'0')
		}
		return out, true
	default:
		return 0, false
	}
}
