-- +goose Up
-- +goose StatementBegin
-- Remove error_message from meetings — errors are tracked in meeting_tasks
DO $$
BEGIN
    IF EXISTS (
        SELECT FROM information_schema.columns
        WHERE table_name = 'meetings' AND column_name = 'error_message'
    ) THEN
        ALTER TABLE meetings DROP COLUMN error_message;
    END IF;
END $$;
-- +goose StatementEnd
