package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadUsage(t *testing.T) {
	if code := run([]string{}); code != 2 {
		t.Fatalf("код %d", code)
	}
}

func TestUploadSendsBinaries(t *testing.T) {
	dir := t.TempDir()
	linux := filepath.Join(dir, "linux")
	windows := filepath.Join(dir, "windows")
	if err := os.WriteFile(linux, []byte("L"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(windows, []byte("W"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotToken, gotVersion string
	var gotLinux, gotWindows []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/releases" || r.Method != http.MethodPost {
			t.Errorf("путь %s %s", r.Method, r.URL.Path)
		}
		gotToken = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		gotVersion = r.FormValue("version")
		read := func(name string) []byte {
			file, _, err := r.FormFile(name)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		gotLinux = read("linux_file")
		gotWindows = read("windows_file")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"release":{"version":"1.2.3","status":"dev"}}`)
	}))
	defer srv.Close()
	code := run([]string{"-server", srv.URL, "-token", "secret-token", "-version", "1.2.3", "-linux", linux, "-windows", windows})
	if code != 0 {
		t.Fatalf("код %d", code)
	}
	if gotToken != "Bearer secret-token" || gotVersion != "1.2.3" || string(gotLinux) != "L" || string(gotWindows) != "W" {
		t.Fatalf("token %s version %s linux %s windows %s", gotToken, gotVersion, gotLinux, gotWindows)
	}
}

func TestUploadConflict(t *testing.T) {
	dir := t.TempDir()
	linux := filepath.Join(dir, "linux")
	if err := os.WriteFile(linux, []byte("L"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"конфликт"}`)
	}))
	defer srv.Close()
	err := upload(srv.URL, "tok", "1.0.0", "", map[string]string{"linux_file": linux})
	if err == nil || !strings.Contains(err.Error(), "уже выпущена") || !strings.Contains(err.Error(), "конфликт") {
		t.Fatalf("ошибка %v", err)
	}
}
