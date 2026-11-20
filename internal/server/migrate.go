package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// PostgresVersion — версия PostgreSQL, к которой подключались тесты.
const PostgresVersion = "16.3"

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	for stmt := range strings.SplitSeq(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция: %w", err)
		}
	}
	return nil
}
