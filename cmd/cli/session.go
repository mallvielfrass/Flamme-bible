package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Session struct {
	Server      string `json:"server"`
	Token       string `json:"token"`
	DeviceID    string `json:"device_id"`
	UserID      string `json:"user_id"`
	DimensionID string `json:"dimension_id"`
}

func settingsPath(goos, env, appData string) (string, error) {
	if env != "" {
		return env, nil
	}
	if goos == "windows" {
		if appData == "" {
			return "", fmt.Errorf("AppData")
		}
		return filepath.Join(appData, "flammebible"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "flammebible"), nil
}

func settingsDirectory() (string, error) {
	return settingsPath(runtime.GOOS, os.Getenv("FLAMMEBIBLE_CONFIG"), os.Getenv("APPDATA"))
}

func sessionFile() (string, error) {
	dir, err := settingsDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func loadSession() (Session, error) {
	path, err := sessionFile()
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Session{}, fmt.Errorf("нет входа")
		}
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return Session{}, err
	}
	if sess.Token == "" || sess.Server == "" {
		return Session{}, fmt.Errorf("нет входа")
	}
	return sess, nil
}

func saveSession(sess Session) error {
	dir, err := settingsDirectory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func newHTTPClient() (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if path := os.Getenv("FLAMMEBIBLE_CA"); path != "" {
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("сертификат")
		}
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		},
	}, nil
}
