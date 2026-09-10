-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_meetings_transcription_fts ON meetings USING GIN (to_tsvector('russian', transcription)) WHERE transcription IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_meetings_summary_fts ON meetings USING GIN (to_tsvector('russian', summary)) WHERE summary IS NOT NULL;

-- +goose StatementEnd
