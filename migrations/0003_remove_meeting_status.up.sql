-- +goose Up
-- +goose StatementBegin
-- Remove status column from meetings — status is now tracked only in meeting_tasks
DO $$
BEGIN
    IF EXISTS (
        SELECT FROM information_schema.columns
        WHERE table_name = 'meetings' AND column_name = 'status'
    ) THEN
        -- First, sync meeting_tasks status from meetings.status for existing records
        UPDATE meeting_tasks mt
        SET status = m.status
        FROM meetings m
        WHERE mt.meeting_id = m.id
          AND mt.status = 'created'
          AND m.status != 'created';

        -- Then drop the column
        ALTER TABLE meetings DROP COLUMN status;
    END IF;
END $$;

-- +goose StatementEnd
