package auth

import "testing"

// TestHashAndVerifyRoundTrip 正しいパスワードで検証が通り、誤りで通らないことを確認する
func TestHashAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		plain     string
		input     string
		wantValid bool
	}{
		{"correct password", "S3cret!Pass", "S3cret!Pass", true},
		{"wrong password", "S3cret!Pass", "S3cret!Pas", false},
		{"empty input", "S3cret!Pass", "", false},
	}

	hashed, err := Hash("S3cret!Pass")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if len(hashed.Hash) != Pbkdf2KeyLen {
		t.Fatalf("hash length got %d want %d", len(hashed.Hash), Pbkdf2KeyLen)
	}
	if len(hashed.Salt) != SaltLen {
		t.Fatalf("salt length got %d want %d", len(hashed.Salt), SaltLen)
	}
	if hashed.Iter != Pbkdf2DefaultIter {
		t.Fatalf("iter got %d want %d", hashed.Iter, Pbkdf2DefaultIter)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Verify(c.input, hashed); got != c.wantValid {
				t.Fatalf("Verify(%q) got %v want %v", c.input, got, c.wantValid)
			}
		})
	}
}

// TestHashRejectsEmpty 空パスワードを拒否することを確認する
func TestHashRejectsEmpty(t *testing.T) {
	t.Parallel()
	if _, err := Hash(""); err == nil {
		t.Fatal("err want non-nil")
	}
}

// TestVerifyWithBrokenHash 不完全な HashedPassword を渡しても安全に false を返す
func TestVerifyWithBrokenHash(t *testing.T) {
	t.Parallel()

	if Verify("any", HashedPassword{}) {
		t.Fatal("empty hash want false")
	}
	if Verify("any", HashedPassword{Hash: []byte{1}, Salt: nil, Iter: 1}) {
		t.Fatal("missing salt want false")
	}
}
