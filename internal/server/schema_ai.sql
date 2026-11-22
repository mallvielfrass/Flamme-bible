-- Настройки ИИ пользователя. id — uuid, не serial: в этой схеме нет SEQUENCE.

CREATE TABLE IF NOT EXISTS ai_providers (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name text NOT NULL,
  base_url text NOT NULL,
  token_enc bytea NOT NULL,
  token_nonce bytea NOT NULL,
  token_last4 text NOT NULL,
  checked_at timestamptz,
  check_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, name)
);

CREATE TABLE IF NOT EXISTS ai_models (
  id uuid PRIMARY KEY,
  provider_id uuid NOT NULL REFERENCES ai_providers (id) ON DELETE CASCADE,
  model text NOT NULL,
  display_name text NOT NULL DEFAULT '',
  can_tools boolean NOT NULL DEFAULT true,
  can_vision boolean NOT NULL DEFAULT false,
  UNIQUE (provider_id, model)
);

CREATE TABLE IF NOT EXISTS ai_task_models (
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  task text NOT NULL CHECK (task IN ('chat', 'vision', 'parse_song', 'title')),
  model_id uuid NOT NULL REFERENCES ai_models (id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, task)
);
