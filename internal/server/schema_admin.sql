-- Очередь модерации: когда материал отправили на проверку.

ALTER TABLE songbooks ADD COLUMN IF NOT EXISTS submitted_at timestamptz;

ALTER TABLE modules ADD COLUMN IF NOT EXISTS submitted_at timestamptz;

UPDATE songbooks
SET submitted_at = COALESCE(reviewed_at, now())
WHERE submitted_at IS NULL AND status IN ('pending', 'approved', 'rejected');

UPDATE modules
SET submitted_at = COALESCE(reviewed_at, updated_at, created_at)
WHERE submitted_at IS NULL AND status IN ('pending', 'approved', 'rejected');
