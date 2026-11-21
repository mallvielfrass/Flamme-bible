package server

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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
	if err := ensureSongPartKey(ctx, db); err != nil {
		return err
	}
	if err := resetVectorColumn(ctx, db, "song_embeddings", "song_embeddings_hnsw"); err != nil {
		return err
	}
	return resetVectorColumn(ctx, db, "bible_passages", "bible_passages_hnsw")
}

// ensureSongPartKey переводит song_embeddings с ключа (songbook_id, fbid)
// на (songbook_id, fbid, part_index). Старый вектор на всю песню нельзя
// разрезать на части, поэтому строки удаляются и собираются заново.
func ensureSongPartKey(ctx context.Context, db *pgxpool.Pool) error {
	var name, cols string
	err := db.QueryRow(ctx, `
		SELECT c.conname, string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_constraint c
		CROSS JOIN LATERAL unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.conrelid = 'song_embeddings'::regclass AND c.contype = 'p'
		GROUP BY c.conname
	`).Scan(&name, &cols)
	if err == pgx.ErrNoRows {
		name, cols = "", ""
	} else if err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if cols == "songbook_id,fbid,part_index" {
		return ensureSongHNSW(ctx, db)
	}
	if _, err := db.Exec(ctx, `DELETE FROM song_embeddings`); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if name != "" {
		if _, err := db.Exec(ctx, `ALTER TABLE song_embeddings DROP CONSTRAINT `+quoteIdent(name)); err != nil {
			return fmt.Errorf("миграция смыслового поиска: %w", err)
		}
	}
	if _, err := db.Exec(ctx, `ALTER TABLE song_embeddings ADD COLUMN IF NOT EXISTS part_index int`); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE song_embeddings ALTER COLUMN part_index SET NOT NULL`); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE song_embeddings ADD PRIMARY KEY (songbook_id, fbid, part_index)`); err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	return ensureSongHNSW(ctx, db)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func ensureSongHNSW(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS song_embeddings_hnsw ON song_embeddings USING hnsw (embedding vector_cosine_ops)`)
	if err != nil {
		return fmt.Errorf("миграция смыслового поиска: %w", err)
	}
	return nil
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
