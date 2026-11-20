package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSession(t *testing.T) {
	srv := newTestServer(t)
	owner := uuid.NewString()
	device := uuid.NewString()
	password := "secret-pass"
	a := registerUser(t, srv, "a@example.com", password, map[string]any{"owner_id": owner, "device_id": device})
	if a.UserID == owner {
		t.Fatal("регистрация взяла владельца из тела")
	}
	if a.DeviceID != device {
		t.Fatalf("устройство %s, ожидалось %s", a.DeviceID, device)
	}
	ctx := context.Background()
	var hash, member string
	if err := testPool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, a.UserID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == password || !checkPassword(hash, password) {
		t.Fatal("пароль хранится не хешем Argon2id")
	}
	var digest []byte
	if err := testPool.QueryRow(ctx, `
		SELECT t.digest, m.user_id::text
		FROM session_tokens t
		JOIN session_families f ON f.id = t.family_id
		JOIN memberships m ON m.user_id = f.user_id
		WHERE f.device_id = $1
	`, device).Scan(&digest, &member); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(a.Token))
	if hex.EncodeToString(digest) == a.Token || string(digest) == a.Token || !bytesEqual(digest, sum[:]) {
		t.Fatal("токен сессии лежит в базе открытым текстом")
	}
	if member != a.UserID {
		t.Fatal("членство записано не на пользователя сессии")
	}
	code, raw := api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "a@example.com", "password": "nope", "device_id": device,
	})
	if code != http.StatusUnauthorized {
		t.Fatalf("неверный пароль %d %s", code, raw)
	}
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "a@example.com", "password": password, "device_id": device, "owner_id": owner,
	})
	if code != http.StatusOK {
		t.Fatalf("login %d %s", code, raw)
	}
	b := registerUser(t, srv, "b@example.com", password, nil)
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "b@example.com", "password": password, "device_id": device,
	})
	if code != http.StatusForbidden {
		t.Fatalf("чужое устройство %d %s", code, raw)
	}
	code, raw = api(t, srv, a.Token, http.MethodPost, "/refresh", map[string]any{"owner_id": owner})
	if code != http.StatusOK {
		t.Fatalf("refresh %d %s", code, raw)
	}
	var step2 struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &step2)
	code, raw = api(t, srv, step2.Token, http.MethodPost, "/refresh", nil)
	if code != http.StatusOK {
		t.Fatalf("второй refresh %d %s", code, raw)
	}
	var step3 struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &step3)
	code, _ = api(t, srv, a.Token, http.MethodPost, "/refresh", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("повтор старого refresh: %d", code)
	}
	code, _ = api(t, srv, step3.Token, http.MethodPost, "/refresh", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("цепочка после повторного refresh не отозвана: %d", code)
	}
	code, raw = api(t, srv, "", http.MethodPost, "/login", map[string]any{
		"email": "a@example.com", "password": password, "device_id": device,
	})
	if code != http.StatusOK {
		t.Fatalf("login после отзыва %d %s", code, raw)
	}
	var again authResp
	mustJSON(t, raw, &again)
	code, raw = api(t, srv, again.Token, http.MethodPost, "/logout", nil)
	if code != http.StatusOK {
		t.Fatalf("logout %d %s", code, raw)
	}
	code, _ = api(t, srv, again.Token, http.MethodPost, "/refresh", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("logout не закрыл refresh: %d", code)
	}
	if b.UserID == "" {
		t.Fatal("второй пользователь не создан")
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
