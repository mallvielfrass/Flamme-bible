package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flammebible/internal/ai/secret"
	"flammebible/internal/server"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("нужен DATABASE_URL")
	}
	cert := os.Getenv("TLS_CERT")
	key := os.Getenv("TLS_KEY")
	if (cert == "") != (key == "") {
		log.Fatal("TLS_CERT и TLS_KEY задаются вместе")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := server.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	if err := server.CleanupSessions(ctx, pool); err != nil {
		log.Fatal(err)
	}
	if dir := os.Getenv("MEDIA_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Fatal(err)
		}
	}
	if dir := os.Getenv("EMBED_MODELS_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Fatal(err)
		}
	}
	api := server.New(pool)
	if dir := os.Getenv("MEDIA_DIR"); dir != "" {
		modules := filepath.Join(dir, "bible-modules")
		if err := os.MkdirAll(modules, 0o750); err != nil {
			log.Fatal(err)
		}
		api.SetModuleDir(modules)
	}
	if _, err := secret.LoadKey(); err != nil {
		if !errors.Is(err, secret.ErrDisabled) {
			log.Fatal(err)
		}
		log.Printf("ИИ выключен")
	}
	if raw := os.Getenv("SESSION_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl < time.Minute {
			log.Fatal("SESSION_TTL: длительность не короче 1m, например 720h")
		}
		api.SetSessionTTL(ttl)
	}
	if trustProxy(os.Getenv("TRUST_PROXY")) {
		api.SetTrustProxy(true)
	}
	if embedURL := strings.TrimSpace(os.Getenv("EMBED_URL")); embedURL != "" {
		api.SetEmbedder(server.NewHTTPEmbedder(embedURL, os.Getenv("EMBED_MODEL")))
		api.SetEmbedConfig(server.EmbedConfigFromEnv())
		api.StartIndexer(context.Background())
		log.Printf("смысловой поиск: %s", api.EmbedModel())
	} else {
		log.Printf("смысловой поиск выключен")
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if cert != "" {
		log.Printf("https %s", addr)
		log.Fatal(httpSrv.ListenAndServeTLS(cert, key))
	}
	log.Printf("http %s", addr)
	log.Fatal(httpSrv.ListenAndServe())
}

func trustProxy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
