-- Очередь эмбеддингов. Источник правды для воркера: рестарт не теряет задания.
-- id — uuid, не bigserial: в этой схеме нет SEQUENCE (см. change_log и тест миграции).

CREATE TABLE IF NOT EXISTS embedding_jobs (
  id uuid PRIMARY KEY,
  object_type text NOT NULL CHECK (object_type IN ('song_part', 'bible_passage')),
  songbook_id uuid,
  fbid text NOT NULL DEFAULT '',
  part_index int NOT NULL DEFAULT 0,
  module_id uuid,
  book int NOT NULL DEFAULT 0,
  chapter int NOT NULL DEFAULT 0,
  verse_start int NOT NULL DEFAULT 0,
  verse_end int NOT NULL DEFAULT 0,
  book_name text NOT NULL DEFAULT '',
  reference text NOT NULL DEFAULT '',
  content_hash text NOT NULL,
  body text NOT NULL,
  model text NOT NULL,
  dimension int NOT NULL,
  target text NOT NULL DEFAULT 'active' CHECK (target IN ('active', 'next')),
  tokens int NOT NULL DEFAULT 1,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'done', 'failed')),
  attempts int NOT NULL DEFAULT 0,
  last_error text NOT NULL DEFAULT '',
  not_before timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (object_type = 'song_part' AND songbook_id IS NOT NULL AND fbid <> '')
    OR (object_type = 'bible_passage' AND module_id IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS embedding_jobs_song_uidx
  ON embedding_jobs (songbook_id, fbid, part_index, target)
  WHERE object_type = 'song_part';

CREATE UNIQUE INDEX IF NOT EXISTS embedding_jobs_passage_uidx
  ON embedding_jobs (module_id, book, chapter, verse_start, target)
  WHERE object_type = 'bible_passage';

CREATE INDEX IF NOT EXISTS embedding_jobs_status_idx ON embedding_jobs (status);

CREATE INDEX IF NOT EXISTS embedding_jobs_songbook_status_idx ON embedding_jobs (songbook_id, status);
