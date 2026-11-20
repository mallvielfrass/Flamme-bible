package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_semantic.sql
var schemaSemanticSQL string

func migrateSemantic(ctx context.Context, db *pgxpool.Pool) error {
	for stmt := range strings.SplitSeq(schemaSemanticSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("миграция смыслового поиска: %w", err)
		}
	}
	return nil
}
