// Package auth 認証に必要なパスワードハッシュ、セッション、CSRF を扱う
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
)

const (
	// Pbkdf2DefaultIter 2024 年の OWASP 推奨値に近い反復回数を採用する
	Pbkdf2DefaultIter = 600_000
	// Pbkdf2KeyLen 出力ハッシュ長 (バイト)
	Pbkdf2KeyLen = 32
	// SaltLen ソルト長 (バイト)
	SaltLen = 16
)

// HashedPassword 保存用のハッシュとソルトと反復回数を束ねる
type HashedPassword struct {
	Hash []byte
	Salt []byte
	Iter int
}

// Hash 平文パスワードから HashedPassword を生成する
// ソルトは crypto/rand で 16 バイト生成する
func Hash(plain string) (HashedPassword, error) {
	if plain == "" {
		return HashedPassword{}, errors.New("password is empty")
	}
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return HashedPassword{}, fmt.Errorf("salt read: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, plain, salt, Pbkdf2DefaultIter, Pbkdf2KeyLen)
	if err != nil {
		return HashedPassword{}, fmt.Errorf("pbkdf2 key: %w", err)
	}
	return HashedPassword{Hash: key, Salt: salt, Iter: Pbkdf2DefaultIter}, nil
}

// Verify 入力パスワードが保存済みハッシュと一致するかを定数時間で比較する
// 反復回数とソルトはパスワード保存時の値をそのまま渡す
func Verify(plain string, h HashedPassword) bool {
	if plain == "" || h.Iter <= 0 || len(h.Salt) == 0 || len(h.Hash) == 0 {
		return false
	}
	calc, err := pbkdf2.Key(sha256.New, plain, h.Salt, h.Iter, len(h.Hash))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(calc, h.Hash) == 1
}
