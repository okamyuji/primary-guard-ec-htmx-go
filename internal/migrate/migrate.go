// Package migrate アプリ起動時にスキーマを適用する最小マイグレータを提供する
// 引数で受け取った dir 配下の *.up.sql を名前順で実行し
// schema_migrations テーブルで適用済みファイル名を追跡する
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Apply dir 配下の *.up.sql を名前順で適用する
// 二度目以降の実行では schema_migrations に記録された適用済みファイルをスキップする
func Apply(ctx context.Context, db *sql.DB, dir string) error {
	if err := ensureTrackingTable(ctx, db); err != nil {
		return fmt.Errorf("migrate ensure table: %w", err)
	}
	files, err := listMigrationFiles(dir)
	if err != nil {
		return fmt.Errorf("migrate list files: %w", err)
	}
	applied, err := loadApplied(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate load applied: %w", err)
	}
	for _, name := range files {
		if applied[name] {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path) //nolint:gosec // 教材用の固定パス読み込み
		if err != nil {
			return fmt.Errorf("migrate read %s: %w", name, err)
		}
		if err := execScript(ctx, db, string(raw)); err != nil {
			return fmt.Errorf("migrate exec %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO schema_migrations (filename) VALUES (?)", name); err != nil {
			return fmt.Errorf("migrate record %s: %w", name, err)
		}
	}
	return nil
}

// ensureTrackingTable schema_migrations テーブルを作成する
func ensureTrackingTable(ctx context.Context, db *sql.DB) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		filename VARCHAR(255) NOT NULL,
		applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		PRIMARY KEY (filename)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`
	_, err := db.ExecContext(ctx, ddl)
	return err
}

// listMigrationFiles dir から .up.sql ファイル名を名前順で返す
func listMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	return files, nil
}

// loadApplied schema_migrations から適用済みファイル名の集合を返す
func loadApplied(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

// execScript 1 ファイル内の複数 SQL ステートメントを分割して順に実行する
func execScript(ctx context.Context, db *sql.DB, script string) error {
	for _, stmt := range splitStatements(script) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// splitStatements セミコロン区切りで SQL を分割する
// 行頭に -- が付くコメント行を除去し、空のステートメントは捨てる
func splitStatements(script string) []string {
	lines := make([]string, 0, 64)
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, line)
	}
	cleaned := strings.Join(lines, "\n")
	parts := strings.Split(cleaned, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
