CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL
);

CREATE TABLE IF NOT EXISTS dimensions (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('personal', 'community')),
  name text NOT NULL DEFAULT '',
  address text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  confession text NOT NULL DEFAULT '',
  description text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS memberships (
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  user_id uuid NOT NULL REFERENCES users (id),
  role text NOT NULL CHECK (role IN ('admin', 'member')),
  excluded boolean NOT NULL DEFAULT false,
  PRIMARY KEY (dimension_id, user_id)
);

CREATE TABLE IF NOT EXISTS devices (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id)
);

CREATE TABLE IF NOT EXISTS session_families (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id),
  device_id uuid NOT NULL REFERENCES devices (id),
  revoked_at timestamptz
);

CREATE TABLE IF NOT EXISTS session_tokens (
  digest bytea PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES session_families (id),
  replaced_at timestamptz,
  expires_at timestamptz
);

CREATE TABLE IF NOT EXISTS invites (
  digest bytea PRIMARY KEY,
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);

CREATE TABLE IF NOT EXISTS songbooks (
  id uuid PRIMARY KEY,
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  fbid text NOT NULL,
  title text NOT NULL DEFAULT '',
  short_name text NOT NULL DEFAULT '',
  publisher text NOT NULL DEFAULT '',
  revision_date text NOT NULL DEFAULT '',
  comment text NOT NULL DEFAULT '',
  header_template jsonb NOT NULL,
  parent_id uuid REFERENCES songbooks (id),
  hidden boolean NOT NULL DEFAULT false,
  publish_requested boolean NOT NULL DEFAULT false,
  public boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS songs (
  songbook_id uuid NOT NULL REFERENCES songbooks (id),
  fbid text NOT NULL,
  title text NOT NULL DEFAULT '',
  alt_title text NOT NULL DEFAULT '',
  composer text NOT NULL DEFAULT '',
  author text NOT NULL DEFAULT '',
  note text NOT NULL DEFAULT '',
  properties text NOT NULL DEFAULT '',
  parts jsonb NOT NULL,
  revision bigint NOT NULL,
  deleted boolean NOT NULL DEFAULT false,
  PRIMARY KEY (songbook_id, fbid)
);

CREATE TABLE IF NOT EXISTS song_order (
  songbook_id uuid PRIMARY KEY REFERENCES songbooks (id),
  revision bigint NOT NULL,
  fbids jsonb NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_counters (
  dimension_id uuid PRIMARY KEY REFERENCES dimensions (id),
  seq bigint NOT NULL
);

-- seq выдаёт приложение из sync_counters. DEFAULT и SEQUENCE нет.
CREATE TABLE IF NOT EXISTS change_log (
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  seq bigint NOT NULL,
  songbook_id uuid NOT NULL REFERENCES songbooks (id),
  body jsonb NOT NULL,
  PRIMARY KEY (dimension_id, seq)
);

CREATE TABLE IF NOT EXISTS operation_results (
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  device_id uuid NOT NULL REFERENCES devices (id),
  operation_id uuid NOT NULL,
  body_hash bytea NOT NULL,
  result jsonb NOT NULL,
  PRIMARY KEY (dimension_id, device_id, operation_id)
);

CREATE TABLE IF NOT EXISTS conflicts (
  id uuid PRIMARY KEY,
  dimension_id uuid NOT NULL REFERENCES dimensions (id),
  songbook_id uuid NOT NULL REFERENCES songbooks (id),
  song_fbid text NOT NULL,
  operation_id uuid NOT NULL,
  server_body jsonb,
  client_body jsonb NOT NULL
);

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS publish_requested boolean NOT NULL DEFAULT false;
ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS public boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS song_hides (
  user_id uuid NOT NULL REFERENCES users (id),
  songbook_id uuid NOT NULL,
  fbid text NOT NULL,
  PRIMARY KEY (user_id, songbook_id, fbid),
  FOREIGN KEY (songbook_id, fbid) REFERENCES songs (songbook_id, fbid)
);

ALTER TABLE session_tokens ADD COLUMN IF NOT EXISTS expires_at timestamptz;

UPDATE session_tokens SET expires_at = now() + interval '720 hours' WHERE expires_at IS NULL;

CREATE INDEX IF NOT EXISTS session_tokens_expires_at ON session_tokens (expires_at);
