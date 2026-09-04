-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meetings_transcription_fts;
DROP INDEX IF EXISTS idx_meetings_summary_fts;
DROP INDEX IF EXISTS idx_meetings_error;

-- +goose StatementEnd
