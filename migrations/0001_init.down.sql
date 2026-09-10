-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meeting_tasks_status;
DROP INDEX IF EXISTS idx_meeting_tasks_meeting_id;
DROP INDEX IF EXISTS idx_meetings_user_created;

DROP TABLE IF EXISTS meeting_tasks;
DROP TABLE IF EXISTS meetings;
DROP TABLE IF EXISTS users;

-- +goose StatementEnd
