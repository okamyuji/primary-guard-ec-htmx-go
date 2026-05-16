// Package main outbox イベントを Primary から取り出して処理するワーカーの起動エントリ
package main

import (
	"log/slog"
	"os"
)

// main エントリポイント。実体は後続ステップで outbox.Run を呼ぶように差し替える
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("primary-guard-ec-htmx-go worker placeholder")
}
