package migrate

import "os"

// writeFileImpl テスト用にファイルを書き出すだけのラッパ
func writeFileImpl(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
