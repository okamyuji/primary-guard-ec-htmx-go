// Package static embed.FS で静的アセットを束ねバイナリ単体で配信できるようにする
package static

import (
	"embed"
	"io/fs"
)

//go:embed styles.css htmx.min.js
var assets embed.FS

// FS 配信用ファイルシステムを返す
// ルートは / 直下に styles.css と htmx.min.js が並ぶ
func FS() fs.FS {
	return assets
}
