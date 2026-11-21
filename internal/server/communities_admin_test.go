package server

import (
	"net/http"
	"testing"
)

func TestCommunityStaff(t *testing.T) {
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	srv := newTestServer(t)
	owner := registerUser(t, srv, "owner@example.com", "secret-pass", nil)
	guest := registerUser(t, srv, "guest@example.com", "secret-pass", nil)
	site := registerUser(t, srv, "admin@example.com", "secret-pass", nil)
	code, raw := api(t, srv, owner.Token, http.MethodPost, "/api/communities", map[string]any{
		"name": "Дом", "address": "ул. Примерная, 1",
	})
	if code != http.StatusOK {
		t.Fatalf("община %d %s", code, raw)
	}
	var community dimensionView
	mustJSON(t, raw, &community)
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities/"+community.ID+"/invites", map[string]any{"ttl_seconds": 86400})
	if code != http.StatusOK {
		t.Fatalf("приглашение %d %s", code, raw)
	}
	var invite struct {
		Token string `json:"token"`
	}
	mustJSON(t, raw, &invite)
	code, raw = api(t, srv, guest.Token, http.MethodPost, "/api/invites/"+invite.Token+"/accept", nil)
	if code != http.StatusOK {
		t.Fatalf("вход %d %s", code, raw)
	}
	code, raw = api(t, srv, site.Token, http.MethodGet, "/api/admin/communities?q=Пример", nil)
	if code != http.StatusOK {
		t.Fatalf("список %d %s", code, raw)
	}
	var page struct {
		Communities []communityRow `json:"communities"`
		Total       int            `json:"total"`
	}
	mustJSON(t, raw, &page)
	if page.Total != 1 || page.Communities[0].Members != 2 {
		t.Fatalf("общины %+v", page)
	}
	code, raw = api(t, srv, site.Token, http.MethodGet, "/api/admin/communities/"+community.ID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("участники сайта %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/communities/"+community.ID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("участники общины %d %s", code, raw)
	}
	var members struct {
		Members []memberRow `json:"members"`
	}
	mustJSON(t, raw, &members)
	if len(members.Members) != 2 {
		t.Fatalf("состав %+v", members.Members)
	}
	code, raw = api(t, srv, guest.Token, http.MethodGet, "/api/communities/"+community.ID+"/members", nil)
	if code != http.StatusForbidden {
		t.Fatalf("гость видит состав %d %s", code, raw)
	}
	code, raw = api(t, srv, site.Token, http.MethodPost, "/api/communities/"+community.ID+"/members/"+guest.UserID+"/exclude", nil)
	if code != http.StatusOK {
		t.Fatalf("исключение %d %s", code, raw)
	}
	code, raw = api(t, srv, site.Token, http.MethodPost, "/api/communities/"+community.ID+"/members/"+guest.UserID+"/include", nil)
	if code != http.StatusOK {
		t.Fatalf("возврат %d %s", code, raw)
	}
	code, raw = api(t, srv, site.Token, http.MethodPost, "/api/communities/"+community.ID+"/members/"+guest.UserID+"/role", map[string]any{"role": "admin"})
	if code != http.StatusOK {
		t.Fatalf("роль %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/api/communities/"+community.ID+"/invites", map[string]any{"ttl_seconds": 3600})
	if code != http.StatusOK {
		t.Fatalf("ещё приглашение %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodGet, "/api/communities/"+community.ID+"/invites", nil)
	if code != http.StatusOK {
		t.Fatalf("приглашения %d %s", code, raw)
	}
	var invites struct {
		Invites []inviteRow `json:"invites"`
	}
	mustJSON(t, raw, &invites)
	if len(invites.Invites) != 1 {
		t.Fatalf("активные %+v", invites.Invites)
	}
	code, raw = api(t, srv, site.Token, http.MethodDelete, "/api/communities/"+community.ID+"/invites/"+invites.Invites[0].Digest, nil)
	if code != http.StatusOK {
		t.Fatalf("отзыв %d %s", code, raw)
	}
	code, raw = api(t, srv, owner.Token, http.MethodPost, "/dimensions/"+community.ID+"/members/"+guest.UserID+"/exclude", nil)
	if code != http.StatusOK {
		t.Fatalf("старый путь %d %s", code, raw)
	}
}
