package render

import (
	"bytes"
	"testing"
	"time"
)

// TestYenFormatting 桁区切りと符号の扱いを確認する
func TestYenFormatting(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   int
		want string
	}{
		{0, "¥0"},
		{42, "¥42"},
		{1234, "¥1,234"},
		{1234567, "¥1,234,567"},
		{-1500, "-¥1,500"},
	}
	for _, c := range cases {
		if got := yen(c.in); got != c.want {
			t.Errorf("yen(%d) got %s want %s", c.in, got, c.want)
		}
	}
}

// TestDatetimeJST JST に変換した形式で出力されることを確認する
func TestDatetimeJST(t *testing.T) {
	t.Parallel()

	utc := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		loc = time.FixedZone("Asia/Tokyo", 9*60*60)
	}
	got := datetime(loc)(utc)
	if got != "2026-05-16 09:00" {
		t.Errorf("got %s want 2026-05-16 09:00", got)
	}
}

// TestDatetimeZeroEmpty zero 値は空文字
func TestDatetimeZeroEmpty(t *testing.T) {
	t.Parallel()

	loc := time.UTC
	if got := datetime(loc)(time.Time{}); got != "" {
		t.Fatalf("zero got %s want empty", got)
	}
}

// TestNewParsesAllTemplates 全テンプレートが parse でき、Page で何かしら出力できる
func TestNewParsesAllTemplates(t *testing.T) {
	t.Parallel()

	r, err := New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	type pageData struct {
		Title     string
		UserID    int64
		IsAdmin   bool
		CSRFToken string
		Flash     any
		Page      int
		HasNext   bool
		Products  []any
		Degraded  bool
	}
	buf := &bytes.Buffer{}
	if err := r.Page(buf, "home.html", pageData{Title: "test"}); err != nil {
		t.Fatalf("page home: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("home output empty")
	}
}
