package server

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMigrateTwice(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	major, version, err := postgresMajor(ctx, testPool)
	if err != nil {
		t.Fatal(err)
	}
	if major < minPostgresMajor {
		t.Fatalf("версия PostgreSQL %q (мажор %d), нужна %d или новее", version, major, minPostgresMajor)
	}
	if majorFromVersionNum(160010) != 16 || majorFromVersionNum(170000) != 17 || majorFromVersionNum(180001) != 18 {
		t.Fatal("server_version_num разбирается не по мажору")
	}
	var sequences int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'S'
	`).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if sequences != 0 {
		t.Fatalf("в схеме есть SEQUENCE: %d", sequences)
	}
	var seqDefault *string
	if err := testPool.QueryRow(ctx, `
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'change_log' AND column_name = 'seq'
	`).Scan(&seqDefault); err != nil {
		t.Fatal(err)
	}
	if seqDefault != nil {
		t.Fatalf("change_log.seq имеет default %s", *seqDefault)
	}
	var plain int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name IN ('password', 'token', 'refresh_token')
	`).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if plain != 0 {
		t.Fatal("в схеме есть открытый пароль или токен")
	}
	id := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`, id, "migrate@example.com", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("повторная миграция не сохранила пользователя: %d", n)
	}
}

func TestEmbeddingDimensionReset(t *testing.T) {
	reset(t)
	ctx := context.Background()
	userID := uuid.NewString()
	dimID := uuid.NewString()
	bookID := uuid.NewString()
	moduleID := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`, userID, "dim@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO dimensions (id, kind) VALUES ($1, 'personal')`, dimID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO songbooks (id, dimension_id, fbid, header_template) VALUES ($1, $2, 'book', '{}')`, bookID, dimID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO songs (songbook_id, fbid, parts, revision) VALUES ($1, 'song', '[]', 1)`, bookID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO modules (id, owner_user_id, short_name) VALUES ($1, $2, 'RST')`, moduleID, userID); err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"song_embeddings_hnsw", "bible_passages_hnsw"} {
		if _, err := testPool.Exec(ctx, `DROP INDEX IF EXISTS `+index); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testPool.Exec(ctx, `ALTER TABLE song_embeddings ALTER COLUMN embedding TYPE vector(384)`); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `ALTER TABLE bible_passages ALTER COLUMN embedding TYPE vector(384)`); err != nil {
		t.Fatal(err)
	}
	old := vectorOnes(384)
	if _, err := testPool.Exec(ctx, `
		INSERT INTO song_embeddings (songbook_id, fbid, part_index, model, text_hash, embedding)
		VALUES ($1, 'song', 0, 'old', 'hash', $2::vector)
	`, bookID, old); err != nil {
		t.Fatal(err)
	}
	passageID := uuid.NewString()
	if _, err := testPool.Exec(ctx, `
		INSERT INTO bible_passages (
			id, module_id, book, chapter, verse_start, verse_end, reference, body, model, text_hash, embedding
		) VALUES ($1, $2, 1, 1, 1, 1, 'Быт 1:1', 'текст', 'old', 'hash', $3::vector)
	`, passageID, moduleID, old); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	if dim := columnDim(t, "song_embeddings"); dim != embedDim {
		t.Fatalf("песни: размерность %d", dim)
	}
	if dim := columnDim(t, "bible_passages"); dim != embedDim {
		t.Fatalf("отрывки: размерность %d", dim)
	}
	var songs, passages int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM song_embeddings`).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM bible_passages`).Scan(&passages); err != nil {
		t.Fatal(err)
	}
	if songs != 0 || passages != 0 {
		t.Fatalf("старые векторы остались: песни %d, отрывки %d", songs, passages)
	}
	fresh := vectorOnes(embedDim)
	if _, err := testPool.Exec(ctx, `
		INSERT INTO song_embeddings (songbook_id, fbid, part_index, model, text_hash, embedding)
		VALUES ($1, 'song', 0, 'new', 'hash', $2::vector)
	`, bookID, fresh); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM song_embeddings`).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if songs != 1 {
		t.Fatalf("повторная миграция стерла актуальный вектор: %d", songs)
	}
}

func TestSongEmbeddingPartKey(t *testing.T) {
	reset(t)
	ctx := context.Background()
	userID := uuid.NewString()
	dimID := uuid.NewString()
	bookID := uuid.NewString()
	if _, err := testPool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`, userID, "part@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO dimensions (id, kind) VALUES ($1, 'personal')`, dimID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO songbooks (id, dimension_id, fbid, header_template) VALUES ($1, $2, 'book', '{}')`, bookID, dimID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO songs (songbook_id, fbid, parts, revision) VALUES ($1, 'song', '[]', 1)`, bookID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `DROP INDEX IF EXISTS song_embeddings_hnsw`); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `DROP TABLE song_embeddings`); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		CREATE TABLE song_embeddings (
		  songbook_id uuid NOT NULL,
		  fbid text NOT NULL,
		  model text NOT NULL,
		  text_hash text NOT NULL,
		  embedding vector(312) NOT NULL,
		  PRIMARY KEY (songbook_id, fbid),
		  FOREIGN KEY (songbook_id, fbid) REFERENCES songs (songbook_id, fbid) ON DELETE CASCADE
		)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		INSERT INTO song_embeddings (songbook_id, fbid, model, text_hash, embedding)
		VALUES ($1, 'song', 'old', 'hash', $2::vector)
	`, bookID, vectorOnes(embedDim)); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	var cols string
	if err := testPool.QueryRow(ctx, `
		SELECT string_agg(a.attname, ',' ORDER BY k.ord)
		FROM pg_constraint c
		CROSS JOIN LATERAL unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.conrelid = 'song_embeddings'::regclass AND c.contype = 'p'
	`).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != "songbook_id,fbid,part_index" {
		t.Fatalf("ключ %s", cols)
	}
	var songs int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM song_embeddings`).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if songs != 0 {
		t.Fatalf("старый вектор остался: %d", songs)
	}
	var indexdef string
	if err := testPool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE indexname = 'song_embeddings_hnsw'`).Scan(&indexdef); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexdef, "hnsw") || !strings.Contains(indexdef, "vector_cosine_ops") {
		t.Fatalf("индекс %s", indexdef)
	}
	if _, err := testPool.Exec(ctx, `
		INSERT INTO song_embeddings (songbook_id, fbid, part_index, model, text_hash, embedding)
		VALUES ($1, 'song', 0, 'new', 'hash', $2::vector)
	`, bookID, vectorOnes(embedDim)); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, testPool); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM song_embeddings`).Scan(&songs); err != nil {
		t.Fatal(err)
	}
	if songs != 1 {
		t.Fatalf("повтор стёр вектор части: %d", songs)
	}
}

func columnDim(t *testing.T, table string) int {
	t.Helper()
	var dim int
	if err := testPool.QueryRow(context.Background(), `
		SELECT atttypmod FROM pg_attribute
		WHERE attrelid = $1::regclass AND attname = 'embedding' AND NOT attisdropped
	`, table).Scan(&dim); err != nil {
		t.Fatal(err)
	}
	return dim
}

func vectorOnes(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "1"
	}
	return "[" + strings.Join(parts, ",") + "]"
}
