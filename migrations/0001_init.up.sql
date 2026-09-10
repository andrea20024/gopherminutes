-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT UNIQUE NOT NULL,
    username    TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meetings (
    id            SERIAL PRIMARY KEY,
    user_id       INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name     TEXT NOT NULL,
    transcription TEXT,
    summary       TEXT,
    gridfs_id     TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS meeting_tasks (
    id            SERIAL PRIMARY KEY,
    meeting_id    INT NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'created',
    error_message TEXT,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_meetings_user_created ON meetings (user_id, created_at DESC);
CREATE INDEX idx_meeting_tasks_meeting_id ON meeting_tasks (meeting_id);
CREATE INDEX idx_meeting_tasks_status ON meeting_tasks (status);

-- +goose StatementEnd
