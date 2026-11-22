package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestListModels(t *testing.T) {
	token := "sk-test-token-1234"
	var auth, target string
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		target = r.URL.String()
		auth = r.Header.Get("Authorization")
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("нет таймаута")
		} else {
			left := time.Until(deadline)
			if left < 8*time.Second || left > 10*time.Second {
				t.Errorf("таймаут %s", left)
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"a"},{"id":" "},{"id":"a"},{"id":"b"}]}`)),
			Request:    r,
		}, nil
	})}
	ids, err := (&Client{BaseURL: "https://example.com/v1/", Token: token, HTTP: httpClient}).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target != "https://example.com/v1/models" {
		t.Fatalf("адрес %s", target)
	}
	if auth != "Bearer "+token {
		t.Fatalf("заголовок %q", auth)
	}
	if strings.Join(ids, ",") != "a,b" {
		t.Fatalf("модели %v", ids)
	}
	if _, err := (&Client{BaseURL: "https://example.com/v1", Token: token}).ListModels(context.Background()); err == nil {
		t.Fatal("пустой клиент принят")
	}
}

func TestListModelsErrorScrub(t *testing.T) {
	token := "sk-test-token-1234"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "нет доступа Bearer "+token+" конец", http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL + "/v1", Token: token, HTTP: srv.Client()}).ListModels(context.Background())
	if err == nil {
		t.Fatal("ждали ошибку")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("токен в ошибке: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "Bearer ***") {
		t.Fatalf("ошибка %s", err.Error())
	}
}

func TestListModelsHTMLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<!doctype html><html><title>Example Domain</title></html>")
	}))
	defer srv.Close()
	_, err := (&Client{BaseURL: srv.URL + "/v1", Token: "sk-test-token-1234", HTTP: srv.Client()}).ListModels(context.Background())
	if err == nil || strings.Contains(err.Error(), "<html") || strings.Contains(err.Error(), "doctype") {
		t.Fatalf("html в ошибке: %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("нет кода: %v", err)
	}
}
