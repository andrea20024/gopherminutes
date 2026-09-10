-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT FROM information_schema.columns
        WHERE table_name = 'meetings' AND column_name = 'error_message'
    ) THEN
        ALTER TABLE meetings ADD COLUMN error_message TEXT;
    END IF;
END $$;
-- +goose StatementEnd
