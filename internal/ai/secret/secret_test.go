package secret

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := bytes.Repeat([]byte{7}, 32)
	t.Setenv("AI_SECRET_KEY", base64.StdEncoding.EncodeToString(key))
	got, err := LoadKey()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, key) {
		t.Fatal("ключ не совпал")
	}
	return got
}

func TestLoadKey(t *testing.T) {
	t.Setenv("AI_SECRET_KEY", "")
	_, err := LoadKey()
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("пустой ключ: %v", err)
	}
	t.Setenv("AI_SECRET_KEY", "   ")
	_, err = LoadKey()
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("пробелы: %v", err)
	}
	t.Setenv("AI_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte("short")))
	_, err = LoadKey()
	if err == nil || errors.Is(err, ErrDisabled) {
		t.Fatalf("короткий ключ: %v", err)
	}
	if strings.Contains(err.Error(), "short") {
		t.Fatal("ошибка содержит значение ключа")
	}
}

func TestSealOpen(t *testing.T) {
	key := testKey(t)
	plain := []byte("sk-test-token-1234")
	aad := "user|provider"
	box, nonce, err := Seal(key, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) != 12 {
		t.Fatalf("nonce %d", len(nonce))
	}
	if bytes.Contains(box, plain) {
		t.Fatal("токен виден в шифртексте")
	}
	got, err := Open(key, box, nonce, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("открылось %q", got)
	}
	box2, nonce2, err := Seal(key, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(nonce, nonce2) || bytes.Equal(box, box2) {
		t.Fatal("nonce не случайный")
	}
	if _, err := Open(key, box, nonce, "user|other"); err == nil {
		t.Fatal("чужой aad принят")
	}
	other := bytes.Repeat([]byte{9}, 32)
	if _, err := Open(other, box, nonce, aad); err == nil {
		t.Fatal("чужой ключ принят")
	}
	bad := append([]byte(nil), box...)
	bad[0] ^= 1
	if _, err := Open(key, bad, nonce, aad); err == nil {
		t.Fatal("битый шифртекст принят")
	}
	if _, err := Open(key, box, nonce[:8], aad); err == nil {
		t.Fatal("короткий nonce принят")
	}
}

func TestLast4AndScrub(t *testing.T) {
	if Last4("sk-test-token-1234") != "1234" || Last4("ab") != "ab" || Last4("") != "" {
		t.Fatal("last4")
	}
	token := "sk-test-token-1234"
	text := Scrub("сбой Bearer "+token+" и ещё "+token, token)
	if strings.Contains(text, token) || !strings.Contains(text, "Bearer ***") {
		t.Fatalf("scrub %q", text)
	}
	long := strings.Repeat("я", 300) + " Bearer secret-value"
	cut := Scrub(long, "")
	if len([]rune(cut)) != 240 {
		t.Fatalf("длина %d", len([]rune(cut)))
	}
	if strings.Contains(cut, "Bearer") {
		t.Fatal("обрезка оставила хвост с Bearer")
	}
}
