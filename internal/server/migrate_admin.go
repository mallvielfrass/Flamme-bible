package server

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_admin.sql
var schemaAdminSQL string

func migrateAdmin(ctx context.Context, db *pgxpool.Pool) error {
	return execStatements(ctx, db, schemaAdminSQL, "админка")
}
