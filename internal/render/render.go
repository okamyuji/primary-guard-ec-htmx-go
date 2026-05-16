// Package render html/template を束ねた薄いレンダラを提供する
// embed.FS 上のテンプレートを起動時に一度だけ ParseFS して再パースを避ける
package render

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"time"
)

//go:embed templates/*.html templates/partials/*.html
var templatesFS embed.FS

// Renderer 解析済みテンプレート集合をラップする
type Renderer struct {
	tmpl *template.Template
	loc  *time.Location
}

// New templates と partials を解析した Renderer を返す
// 失敗したらアプリ起動を中断する想定なので呼び出し側で fatal にする
func New() (*Renderer, error) {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		loc = time.UTC
	}
	funcs := template.FuncMap{
		"yen":      yen,
		"datetime": datetime(loc),
		"add":      func(a, b int) int { return a + b },
		"sub":      func(a, b int) int { return a - b },
		"csrfField": func(token string) template.HTML {
			return template.HTML(`<input type="hidden" name="csrf" value="` + template.HTMLEscapeString(token) + `">`) //nolint:gosec // 既にエスケープ済みのトークンのみを差し込む
		},
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(templatesFS,
		"templates/*.html",
		"templates/partials/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("render parse: %w", err)
	}
	return &Renderer{tmpl: tmpl, loc: loc}, nil
}

// Page ベースレイアウトを使ってページテンプレートを出力する
// 名前にはページテンプレートのファイル名 (拡張子つき) を渡す
func (r *Renderer) Page(w io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("render page %s: %w", name, err)
	}
	return nil
}

// Partial パーシャル単独で出力する
// HTMX の部分更新ハンドラから使う
func (r *Renderer) Partial(w io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("render partial %s: %w", name, err)
	}
	return nil
}

// yen 整数の円表記を作る
// 1234 -> "¥1,234"
func yen(v int) string {
	negative := v < 0
	n := v
	if negative {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if negative {
			return "-¥" + s
		}
		return "¥" + s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if negative {
		return "-¥" + string(out)
	}
	return "¥" + string(out)
}

// datetime クロージャで JST フォーマッタを返す
func datetime(loc *time.Location) func(t time.Time) string {
	return func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.In(loc).Format("2006-01-02 15:04")
	}
}
