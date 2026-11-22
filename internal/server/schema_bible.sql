-- Модули Библии, которые клиент заливает целиком.
-- Пустой content_hash — карточка без файла (старый POST /api/modules).

ALTER TABLE modules ADD COLUMN IF NOT EXISTS content_hash text NOT NULL DEFAULT '';

ALTER TABLE modules ADD COLUMN IF NOT EXISTS content_bytes bigint NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS modules_content_hash_idx ON modules (content_hash) WHERE content_hash <> '';
