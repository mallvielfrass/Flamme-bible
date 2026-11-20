package server

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestDimensionInvite(t *testing.T) {
	srv := newTestServer(t)
	password := "secret-pass"
	a := registerUser(t, srv, "a@example.com", password, nil)
	b := registerUser(t, srv, "b@example.com", password, nil)
	c := registerUser(t, srv, "c@example.com", password, nil)
	code, raw := api(t, srv, a.Token, http.MethodGet, "/dimensions/"+a.DimensionID, nil)
	if code != http.StatusOK {
		t.Fatalf("личное пространство %d %s", code, raw)
	}
	var personal dimensionView
	mustJSON(t, raw, &personal)
	if personal.Kind != "personal" {
		t.Fatalf("вид личного пространства %s", personal.Kind)
	}
	code, _ = api(t, srv, b.Token, http.MethodGet, "/dimensions/"+a.DimensionID, nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужое личное пространство %d", code)
	}
	owner := uuid.NewString()
	code, raw = api(t, srv, a.Token, http.MethodPost, "/dimensions", map[string]any{
		"name": "Община", "address": "ул", "email": "c@x", "confession": "нет", "description": "описание", "owner_id": owner,
	})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community dimensionView
	mustJSON(t, raw, &community)
	var admin string
	if err := testPool.QueryRow(context.Background(), `SELECT user_id::text FROM memberships WHERE dimension_id = $1 AND role = 'admin'`, community.ID).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if admin != a.UserID || admin == owner {
		t.Fatalf("админ общины %s", admin)
	}
	code, _ = api(t, srv, a.Token, http.MethodPost, "/dimensions/"+a.DimensionID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusBadRequest {
		t.Fatalf("приглашение в личное пространство %d", code)
	}
	code, raw = api(t, srv, a.Token, http.MethodPost, "/dimensions/"+community.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("приглашение %d %s", code, raw)
	}
	var invite struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &invite)
	code, raw = api(t, srv, b.Token, http.MethodPost, "/invites/"+url.PathEscape(invite.Token)+"/accept", nil)
	if code != http.StatusOK {
		t.Fatalf("вступление %d %s", code, raw)
	}
	code, _ = api(t, srv, b.Token, http.MethodGet, "/dimensions/"+community.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("участник не читает общину: %d", code)
	}
	code, _ = api(t, srv, b.Token, http.MethodPost, "/dimensions/"+community.ID+"/members/"+a.UserID+"/exclude", nil)
	if code != http.StatusForbidden {
		t.Fatalf("member исключает admin: %d", code)
	}
	code, _ = api(t, srv, c.Token, http.MethodPost, "/invites/"+url.PathEscape(invite.Token)+"/accept", nil)
	if code != http.StatusNotFound {
		t.Fatalf("повторное приглашение %d", code)
	}
	code, raw = api(t, srv, a.Token, http.MethodPost, "/dimensions/"+community.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("второе приглашение %d %s", code, raw)
	}
	var expired struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &expired)
	if _, err := testPool.Exec(context.Background(), `UPDATE invites SET expires_at = now() - interval '1 minute' WHERE digest = $1`, tokenDigest(expired.Token)); err != nil {
		t.Fatal(err)
	}
	code, _ = api(t, srv, c.Token, http.MethodPost, "/invites/"+url.PathEscape(expired.Token)+"/accept", nil)
	if code != http.StatusNotFound {
		t.Fatalf("просроченное приглашение %d", code)
	}
	code, raw = api(t, srv, a.Token, http.MethodPost, "/dimensions/"+community.ID+"/members/"+b.UserID+"/exclude", nil)
	if code != http.StatusOK {
		t.Fatalf("исключение %d %s", code, raw)
	}
	code, _ = api(t, srv, b.Token, http.MethodGet, "/dimensions/"+community.ID, nil)
	if code != http.StatusForbidden {
		t.Fatalf("исключённый читает общину: %d", code)
	}
	code, _ = api(t, srv, a.Token, http.MethodGet, "/dimensions/"+community.ID, nil)
	if code != http.StatusOK {
		t.Fatalf("админ после исключения %d", code)
	}
	code, _ = api(t, srv, a.Token, http.MethodGet, "/dimensions/"+uuid.NewString(), nil)
	if code != http.StatusNotFound {
		t.Fatalf("чужой id %d", code)
	}
}
