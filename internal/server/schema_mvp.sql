-- Дополнение MVP. Отдельный файл, чтобы параллельная правка сессий не переписывала эти операторы.

ALTER TABLE users ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'user';

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'draft';

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS moderation_note text NOT NULL DEFAULT '';

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS reviewed_by uuid;

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;

UPDATE songbooks SET status = 'approved' WHERE public AND status = 'draft';

UPDATE songbooks SET status = 'pending' WHERE publish_requested AND NOT public AND status = 'draft';

ALTER TABLE songbooks DROP CONSTRAINT IF EXISTS songbooks_status_check;

ALTER TABLE songbooks ADD CONSTRAINT songbooks_status_check CHECK (status IN ('draft', 'pending', 'approved', 'rejected'));

ALTER TABLE songbooks DROP CONSTRAINT IF EXISTS songbooks_reviewed_by_fkey;

ALTER TABLE songbooks ADD CONSTRAINT songbooks_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES users (id);

CREATE TABLE IF NOT EXISTS modules (
  id uuid PRIMARY KEY,
  owner_user_id uuid REFERENCES users (id),
  dimension_id uuid REFERENCES dimensions (id),
  short_name text NOT NULL,
  title text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'draft',
  moderation_note text NOT NULL DEFAULT '',
  reviewed_by uuid REFERENCES users (id),
  reviewed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT modules_scope CHECK (
    (owner_user_id IS NOT NULL AND dimension_id IS NULL)
    OR (owner_user_id IS NULL AND dimension_id IS NOT NULL)
  ),
  CONSTRAINT modules_status_check CHECK (status IN ('draft', 'pending', 'approved', 'rejected'))
);

CREATE TABLE IF NOT EXISTS plans (
  id uuid PRIMARY KEY,
  owner_user_id uuid REFERENCES users (id),
  dimension_id uuid REFERENCES dimensions (id),
  title text NOT NULL DEFAULT '',
  version bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted boolean NOT NULL DEFAULT false,
  CONSTRAINT plans_scope CHECK (
    (owner_user_id IS NOT NULL AND dimension_id IS NULL)
    OR (owner_user_id IS NULL AND dimension_id IS NOT NULL)
  )
);

CREATE TABLE IF NOT EXISTS plan_items (
  id uuid PRIMARY KEY,
  plan_id uuid NOT NULL REFERENCES plans (id) ON DELETE CASCADE,
  position int NOT NULL,
  kind text NOT NULL,
  song_id text NOT NULL DEFAULT '',
  songbook_id uuid,
  title text NOT NULL DEFAULT '',
  module_short_name text NOT NULL DEFAULT '',
  module_short_name_second text NOT NULL DEFAULT '',
  book int NOT NULL DEFAULT 0,
  chapter int NOT NULL DEFAULT 0,
  verse int NOT NULL DEFAULT 0,
  caption text NOT NULL DEFAULT '',
  reference text NOT NULL DEFAULT '',
  quote text NOT NULL DEFAULT '',
  class text NOT NULL DEFAULT '',
  CONSTRAINT plan_items_kind_check CHECK (kind IN ('song', 'bible', 'text'))
);

CREATE TABLE IF NOT EXISTS client_releases (
  version text PRIMARY KEY,
  status text NOT NULL,
  linux_sha256 text NOT NULL DEFAULT '',
  linux_size bigint NOT NULL DEFAULT 0,
  windows_sha256 text NOT NULL DEFAULT '',
  windows_size bigint NOT NULL DEFAULT 0,
  macos_sha256 text NOT NULL DEFAULT '',
  macos_size bigint NOT NULL DEFAULT 0,
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  CONSTRAINT client_releases_status_check CHECK (status IN ('dev', 'released'))
);

CREATE TABLE IF NOT EXISTS app_settings (
  key text PRIMARY KEY,
  value text NOT NULL
);

INSERT INTO app_settings (key, value)
VALUES ('active_client_version', '')
ON CONFLICT (key) DO NOTHING;

CREATE INDEX IF NOT EXISTS plans_owner_idx ON plans (owner_user_id);

CREATE INDEX IF NOT EXISTS plans_dimension_idx ON plans (dimension_id);

CREATE INDEX IF NOT EXISTS modules_status_idx ON modules (status);

CREATE INDEX IF NOT EXISTS songbooks_status_idx ON songbooks (status);
