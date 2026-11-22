package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_bible.sql
var schemaBibleSQL string

func migrateBibleModules(ctx context.Context, db *pgxpool.Pool) error {
	for stmt := range strings.SplitSeq(schemaBibleSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция модулей Библии: %w", err)
		}
	}
	return nil
}
