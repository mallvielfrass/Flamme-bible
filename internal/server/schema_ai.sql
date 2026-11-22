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

-- Чаты хранятся постоянно: срока жизни у сообщений нет.
CREATE TABLE IF NOT EXISTS ai_chats (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  title text NOT NULL DEFAULT '',
  model_id uuid REFERENCES ai_models (id) ON DELETE SET NULL,
  pinned boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ai_chats_user_list ON ai_chats (user_id, pinned DESC, updated_at DESC);

CREATE TABLE IF NOT EXISTS ai_messages (
  id uuid PRIMARY KEY,
  chat_id uuid NOT NULL REFERENCES ai_chats (id) ON DELETE CASCADE,
  seq int NOT NULL,
  role text NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
  content text NOT NULL DEFAULT '',
  tool_calls jsonb,
  tool_call_id text,
  cards jsonb,
  images jsonb,
  model text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'done' CHECK (status IN ('done', 'stopped', 'error')),
  prompt_tokens int,
  completion_tokens int,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (chat_id, seq)
);

INSERT INTO app_settings (key, value) VALUES
  ('ai_enabled', 'true'),
  ('ai_msg_per_10min', '0'),
  ('ai_msg_per_day', '0'),
  ('ai_max_tool_calls', '8')
ON CONFLICT (key) DO NOTHING;
