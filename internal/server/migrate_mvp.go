package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_mvp.sql
var schemaMVPSQL string

func migrateMVP(ctx context.Context, db *pgxpool.Pool) error {
	for stmt := range strings.SplitSeq(schemaMVPSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция mvp: %w", err)
		}
	}
	return nil
}
