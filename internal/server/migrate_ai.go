package server

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_ai.sql
var schemaAISQL string

func migrateAI(ctx context.Context, db *pgxpool.Pool) error {
	return execStatements(ctx, db, schemaAISQL, "настройки ИИ")
}
