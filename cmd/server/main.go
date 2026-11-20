package main

import (
	"context"
	"log"
	"net/http"
	"os"

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
	if cert == "" || key == "" {
		log.Fatal("нужны TLS_CERT и TLS_KEY")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(http.ListenAndServeTLS(addr, cert, key, server.New(pool).Handler()))
}
