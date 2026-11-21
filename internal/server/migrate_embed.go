package server

import (
	"context"
	_ "embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema_embed_queue.sql
var schemaEmbedQueueSQL string

func migrateEmbedQueue(ctx context.Context, db *pgxpool.Pool) error {
	return execStatements(ctx, db, schemaEmbedQueueSQL, "очередь эмбеддингов")
}
