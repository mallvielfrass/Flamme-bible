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
	if err := resetVectorColumn(ctx, db, "song_embeddings", "song_embeddings_hnsw"); err != nil {
		return err
	}
	return resetVectorColumn(ctx, db, "bible_passages", "bible_passages_hnsw")
}

// resetVectorColumn стирает векторы, если колонка собрана под другую модель.
// Строки vector(384) нельзя записать в vector(312), поэтому они удаляются, затем меняется тип колонки.
func resetVectorColumn(ctx context.Context, db *pgxpool.Pool, table, index string) error {
	var dim int
	if err := db.QueryRow(ctx, `
		SELECT atttypmod FROM pg_attribute
		WHERE attrelid = $1::regclass AND attname = 'embedding' AND NOT attisdropped
	`, table).Scan(&dim); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if dim == embedDim {
		return nil
	}
	if _, err := db.Exec(ctx, `DROP INDEX IF EXISTS `+index); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM `+table); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if _, err := db.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN embedding TYPE vector(%d)`, table, embedDim)); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS `+index+` ON `+table+` USING hnsw (embedding vector_cosine_ops)`); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	return nil
}
