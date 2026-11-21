package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_items_time.sql
var schemaItemsTimeSQL string

//go:embed schema_sections.sql
var schemaSectionsSQL string

func migratePlansUI(ctx context.Context, db *pgxpool.Pool) error {
	if err := execStatements(ctx, db, schemaItemsTimeSQL, "времена пунктов"); err != nil {
		return err
	}
	return execStatements(ctx, db, schemaSectionsSQL, "разделы сборников")
}

func execStatements(ctx context.Context, db *pgxpool.Pool, sql, name string) error {
	for stmt := range strings.SplitSeq(sql, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция %s: %w", name, err)
		}
	}
	return nil
}
