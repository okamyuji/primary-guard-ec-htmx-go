package migrate

import (
	"strings"
	"testing"
)

// TestSplitStatementsRemovesComments コメント行が分割結果に含まれないことを確認する
func TestSplitStatementsRemovesComments(t *testing.T) {
	t.Parallel()

	script := `-- ヘッダコメント
CREATE TABLE a (
    id INT PRIMARY KEY
);
-- 中間コメント
INSERT INTO a VALUES (1);
`

	got := splitStatements(script)
	if len(got) != 2 {
		t.Fatalf("statements got %d want 2: %v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "CREATE TABLE") {
		t.Errorf("statement 0 unexpected: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "INSERT INTO") {
		t.Errorf("statement 1 unexpected: %q", got[1])
	}
}

// TestSplitStatementsDropsEmpty 空文を捨てることを確認する
func TestSplitStatementsDropsEmpty(t *testing.T) {
	t.Parallel()

	script := ";;SELECT 1;;"
	got := splitStatements(script)
	if len(got) != 1 || got[0] != "SELECT 1" {
		t.Fatalf("got %v want [SELECT 1]", got)
	}
}

// TestListMigrationFilesSorted ファイルが名前順で返ることを確認する
func TestListMigrationFilesSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	names := []string{"0002_b.up.sql", "0001_a.up.sql", "0003_c.up.sql", "ignored.txt"}
	for _, n := range names {
		path := dir + "/" + n
		if err := writeFile(t, path, "-- empty\n"); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got, err := listMigrationFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"0001_a.up.sql", "0002_b.up.sql", "0003_c.up.sql"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d got %s want %s", i, got[i], want[i])
		}
	}
}

// writeFile テスト用のファイル書き出しヘルパ
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return writeFileImpl(path, content)
}
