-- Смысловой поиск. vector(384) — модель intfloat/multilingual-e5-small.
-- Другая размерность сюда не встанет: нужна новая колонка и полная переиндексация.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS song_embeddings (
  songbook_id uuid NOT NULL,
  fbid text NOT NULL,
  model text NOT NULL,
  text_hash text NOT NULL,
  embedding vector(384) NOT NULL,
  PRIMARY KEY (songbook_id, fbid),
  FOREIGN KEY (songbook_id, fbid) REFERENCES songs (songbook_id, fbid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS song_embeddings_hnsw
  ON song_embeddings USING hnsw (embedding vector_cosine_ops);

CREATE TABLE IF NOT EXISTS module_verses (
  module_id uuid NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
  book int NOT NULL,
  book_name text NOT NULL DEFAULT '',
  chapter int NOT NULL,
  verse int NOT NULL,
  body text NOT NULL,
  PRIMARY KEY (module_id, book, chapter, verse)
);

CREATE TABLE IF NOT EXISTS bible_passages (
  id uuid PRIMARY KEY,
  module_id uuid NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
  book int NOT NULL,
  book_name text NOT NULL DEFAULT '',
  chapter int NOT NULL,
  verse_start int NOT NULL,
  verse_end int NOT NULL,
  reference text NOT NULL,
  body text NOT NULL,
  model text NOT NULL,
  text_hash text NOT NULL,
  embedding vector(384) NOT NULL,
  UNIQUE (module_id, book, chapter, verse_start)
);

CREATE INDEX IF NOT EXISTS bible_passages_hnsw
  ON bible_passages USING hnsw (embedding vector_cosine_ops);
