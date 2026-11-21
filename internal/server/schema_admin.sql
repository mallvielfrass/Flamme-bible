-- Очередь модерации: когда материал отправили на проверку.

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS submitted_at timestamptz;

ALTER TABLE modules ADD COLUMN IF NOT EXISTS submitted_at timestamptz;

UPDATE songbooks
SET submitted_at = COALESCE(reviewed_at, now())
WHERE submitted_at IS NULL AND status IN ('pending', 'approved', 'rejected');

UPDATE modules
SET submitted_at = COALESCE(reviewed_at, updated_at, created_at)
WHERE submitted_at IS NULL AND status IN ('pending', 'approved', 'rejected');

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'moderator', 'admin'));

ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE users ADD COLUMN IF NOT EXISTS blocked_at timestamptz;

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_at timestamptz;

ALTER TABLE songbooks DROP CONSTRAINT IF EXISTS songbooks_reviewed_by_fkey;

ALTER TABLE songbooks ADD CONSTRAINT songbooks_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES users (id) ON DELETE SET NULL;

ALTER TABLE modules DROP CONSTRAINT IF EXISTS modules_reviewed_by_fkey;

ALTER TABLE modules ADD CONSTRAINT modules_reviewed_by_fkey FOREIGN KEY (reviewed_by) REFERENCES users (id) ON DELETE SET NULL;

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS hidden_at timestamptz;
