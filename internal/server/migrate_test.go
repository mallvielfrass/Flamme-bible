package server

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMigrateTwice(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := testPool.QueryRow(ctx, `SHOW server_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, PostgresVersion) {
		t.Fatalf("версия PostgreSQL %q, зафиксирована %s", version, PostgresVersion)
	}
	var sequences int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'S'
	`).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if sequences != 0 {
		t.Fatalf("в схеме есть SEQUENCE: %d", sequences)
	}
	var seqDefault *string
	if err := testPool.QueryRow(ctx, `
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'change_log' AND column_name = 'seq'
	`).Scan(&seqDefault); err != nil {
		t.Fatal(err)
	}
	if seqDefault != nil {
		t.Fatalf("change_log.seq имеет default %s", *seqDefault)
	}
	var plain int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name IN ('password', 'token', 'refresh_token')
	`).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if plain != 0 {
		t.Fatal("в схеме есть открытый пароль или токен")
	}
	id := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`, id, "migrate@example.com", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("повторная миграция не сохранила пользователя: %d", n)
	}
}
