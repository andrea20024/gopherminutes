-- +goose Down
-- +goose StatementBegin
-- Restore status column to meetings (rollback)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT FROM information_schema.columns
        WHERE table_name = 'meetings' AND column_name = 'status'
    ) THEN
        ALTER TABLE meetings ADD COLUMN status TEXT NOT NULL DEFAULT 'created';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_meetings_error;

-- +goose StatementEnd
