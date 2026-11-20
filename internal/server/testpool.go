package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenTestPool поднимает отдельную базу на том же сервере, что и baseURL.
// Пакеты тестов иначе делят одну схему и при go test ./... затирают друг друга.
func OpenTestPool(ctx context.Context, baseURL string) (*pgxpool.Pool, func(), error) {
	cfg, err := pgxpool.ParseConfig(baseURL)
	if err != nil {
		return nil, nil, err
	}
	adminCfg := cfg.Copy()
	adminCfg.ConnConfig.Database = "postgres"
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		return nil, nil, err
	}
	name := fmt.Sprintf("flamme_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		return nil, nil, err
	}
	testCfg := cfg.Copy()
	testCfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, testCfg)
	if err != nil {
		dropTestDB(ctx, admin, name)
		admin.Close()
		return nil, nil, err
	}
	if err := Migrate(ctx, pool); err != nil {
		pool.Close()
		dropTestDB(ctx, admin, name)
		admin.Close()
		return nil, nil, err
	}
	cleanup := func() {
		pool.Close()
		dropTestDB(context.Background(), admin, name)
		admin.Close()
	}
	return pool, cleanup, nil
}

func dropTestDB(ctx context.Context, admin *pgxpool.Pool, name string) {
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
}
