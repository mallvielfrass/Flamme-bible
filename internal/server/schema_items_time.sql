ALTER TABLE plan_items ADD COLUMN IF NOT EXISTS created_at timestamptz;

ALTER TABLE plan_items ADD COLUMN IF NOT EXISTS updated_at timestamptz;

UPDATE plan_items AS i
SET created_at = p.updated_at,
    updated_at = p.updated_at
FROM plans AS p
WHERE i.plan_id = p.id
  AND i.created_at IS NULL;

ALTER TABLE plan_items ALTER COLUMN created_at SET DEFAULT now();

ALTER TABLE plan_items ALTER COLUMN updated_at SET DEFAULT now();

ALTER TABLE plan_items ALTER COLUMN created_at SET NOT NULL;

ALTER TABLE plan_items ALTER COLUMN updated_at SET NOT NULL;
