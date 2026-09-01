package storage

import (
	"context"
	"fmt"
	"time"
)

// MeetingStatus represents the processing status of a meeting.
type MeetingStatus string

const (
	StatusCreated     MeetingStatus = "created"
	StatusProcessing  MeetingStatus = "processing"
	StatusTranscribed MeetingStatus = "transcribed"
	StatusSummarized  MeetingStatus = "summarized"
	StatusCompleted   MeetingStatus = "completed"
	StatusFailed      MeetingStatus = "failed"
)

// Meeting represents a recorded meeting with its processing state.
// Status is derived from the latest MeetingTask (not stored in meetings table).
type Meeting struct {
	ID            int           `json:"id"`
	UserID        int           `json:"user_id"`
	FileName      string        `json:"file_name"`
	Transcription *string       `json:"transcription,omitempty"`
	Summary       *string       `json:"summary,omitempty"`
	Status        MeetingStatus `json:"status"`
	ErrorMessage  *string       `json:"error_message,omitempty"`
	GridFSID      *string       `json:"gridfs_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// MeetingTask represents a background processing task for a meeting.
type MeetingTask struct {
	ID           int           `json:"id"`
	MeetingID    int           `json:"meeting_id"`
	Status       MeetingStatus `json:"status"`
	ErrorMessage *string       `json:"error_message,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// MeetingSummary is used in list/search results for brevity.
type MeetingSummary struct {
	ID           int           `json:"id"`
	FileName     string        `json:"file_name"`
	Status       MeetingStatus `json:"status"`
	Summary      *string       `json:"summary,omitempty"`
	ErrorMessage *string       `json:"error_message,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// MeetingRepo handles meeting and task data operations.
type MeetingRepo struct {
	*Repository
}

// NewMeetingRepo creates a new MeetingRepo.
func NewMeetingRepo(r *Repository) *MeetingRepo {
	return &MeetingRepo{Repository: r}
}

// CreateMeetingWithTask creates a meeting and its initial task in a transaction.
func (r *MeetingRepo) CreateMeetingWithTask(ctx context.Context, userID int, fileName string) (*Meeting, *MeetingTask, error) {
	var meeting Meeting
	var task MeetingTask

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", err)
	}

	err = r.TxQueryRow(tx, `
		INSERT INTO meetings (user_id, file_name) VALUES ($1, $2)
		RETURNING id, user_id, file_name, created_at, updated_at
	`, userID, fileName).Scan(
		&meeting.ID, &meeting.UserID, &meeting.FileName,
		&meeting.CreatedAt, &meeting.UpdatedAt,
	)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return nil, nil, fmt.Errorf("insert meeting: %w (rollback: %v)", err, rbErr)
		}
		return nil, nil, fmt.Errorf("insert meeting: %w", err)
	}

	err = r.TxQueryRow(tx, `
		INSERT INTO meeting_tasks (meeting_id, status) VALUES ($1, $2)
		RETURNING id, meeting_id, status, created_at, updated_at
	`, meeting.ID, string(StatusCreated)).Scan(
		&task.ID, &task.MeetingID, &task.Status,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return nil, nil, fmt.Errorf("insert task: %w (rollback: %v)", err, rbErr)
		}
		return nil, nil, fmt.Errorf("insert task: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Set status from the created task
	meeting.Status = task.Status

	return &meeting, &task, nil
}

// UpdateMeetingStatus updates the latest task status for a meeting.
// Status is now tracked in meeting_tasks, not in meetings.
func (r *MeetingRepo) UpdateMeetingStatus(ctx context.Context, meetingID int, status MeetingStatus, errMsg *string) error {
	// Get latest task ID
	var taskID int
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM meeting_tasks WHERE meeting_id = $1 ORDER BY created_at DESC LIMIT 1
	`, meetingID).Scan(&taskID)
	if err != nil {
		return fmt.Errorf("find latest task: %w", err)
	}

	// Update the task
	var query string
	var args []interface{}

	if errMsg != nil {
		query = `UPDATE meeting_tasks SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`
		args = []interface{}{string(status), *errMsg, taskID}
	} else {
		query = `UPDATE meeting_tasks SET status = $1, updated_at = NOW() WHERE id = $2`
		args = []interface{}{string(status), taskID}
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no task found for meeting %d", meetingID)
	}
	return nil
}

// UpdateTaskStatus updates the status of a meeting task.
func (r *MeetingRepo) UpdateTaskStatus(ctx context.Context, taskID int, status MeetingStatus, errMsg *string) error {
	query := `UPDATE meeting_tasks SET status = $1, updated_at = NOW() WHERE id = $2`
	args := []interface{}{string(status), taskID}

	if errMsg != nil {
		query = `UPDATE meeting_tasks SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`
		args = []interface{}{string(status), *errMsg, taskID}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update task status: %w", err)
	}
	return nil
}

// SaveTranscription saves the transcription text for a meeting and updates task status.
func (r *MeetingRepo) SaveTranscription(ctx context.Context, meetingID int, transcription string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meetings SET transcription = $1, updated_at = NOW() WHERE id = $2
	`, transcription, meetingID)
	if err != nil {
		return fmt.Errorf("save transcription: %w", err)
	}

	// Update latest task status to transcribed
	_, err = r.db.ExecContext(ctx, `
		UPDATE meeting_tasks SET status = $1, updated_at = NOW()
		WHERE id = (SELECT id FROM meeting_tasks WHERE meeting_id = $2 ORDER BY created_at DESC LIMIT 1)
	`, string(StatusTranscribed), meetingID)
	if err != nil {
		return fmt.Errorf("update task status to transcribed: %w", err)
	}
	return nil
}

// SaveSummary saves the summary text for a meeting and updates task status.
func (r *MeetingRepo) SaveSummary(ctx context.Context, meetingID int, summary string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meetings SET summary = $1, updated_at = NOW() WHERE id = $2
	`, summary, meetingID)
	if err != nil {
		return fmt.Errorf("save summary: %w", err)
	}

	// Update latest task status to summarized
	_, err = r.db.ExecContext(ctx, `
		UPDATE meeting_tasks SET status = $1, updated_at = NOW()
		WHERE id = (SELECT id FROM meeting_tasks WHERE meeting_id = $2 ORDER BY created_at DESC LIMIT 1)
	`, string(StatusSummarized), meetingID)
	if err != nil {
		return fmt.Errorf("update task status to summarized: %w", err)
	}
	return nil
}

// CompleteMeeting marks the latest task for a meeting as completed.
func (r *MeetingRepo) CompleteMeeting(ctx context.Context, meetingID int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meeting_tasks SET status = $1, updated_at = NOW()
		WHERE id = (SELECT id FROM meeting_tasks WHERE meeting_id = $2 ORDER BY created_at DESC LIMIT 1)
	`, string(StatusCompleted), meetingID)
	if err != nil {
		return fmt.Errorf("complete task: %w", err)
	}
	return nil
}

// GetMeeting retrieves a meeting by ID with its latest task status and error.
func (r *MeetingRepo) GetMeeting(ctx context.Context, meetingID int) (*Meeting, error) {
	m := &Meeting{}
	err := r.db.QueryRowContext(ctx, `
		SELECT m.id, m.user_id, m.file_name, m.transcription, m.summary,
		       m.gridfs_id, m.created_at, m.updated_at,
		       t.status, t.error_message
		FROM meetings m
		LEFT JOIN LATERAL (
			SELECT status, error_message FROM meeting_tasks WHERE meeting_id = m.id ORDER BY created_at DESC LIMIT 1
		) t ON true
		WHERE m.id = $1
	`, meetingID).Scan(
		&m.ID, &m.UserID, &m.FileName,
		&m.Transcription, &m.Summary,
		&m.GridFSID, &m.CreatedAt, &m.UpdatedAt,
		&m.Status, &m.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("get meeting: %w", err)
	}
	return m, nil
}

// ListMeetingsByUser returns all meetings for a user with latest task status and error, ordered by creation date (newest first).
func (r *MeetingRepo) ListMeetingsByUser(ctx context.Context, userID int) ([]*MeetingSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.file_name, t.status, m.summary, t.error_message, m.created_at, m.updated_at
		FROM meetings m
		LEFT JOIN LATERAL (
			SELECT status, error_message FROM meeting_tasks WHERE meeting_id = m.id ORDER BY created_at DESC LIMIT 1
		) t ON true
		WHERE m.user_id = $1
		ORDER BY m.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list meetings: %w", err)
	}
	defer rows.Close()

	var meetings []*MeetingSummary
	for rows.Next() {
		m := &MeetingSummary{}
		err := rows.Scan(&m.ID, &m.FileName, &m.Status, &m.Summary, &m.ErrorMessage, &m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

// SearchMeetingsByKeyword searches meetings by keyword in transcription or summary.
func (r *MeetingRepo) SearchMeetingsByKeyword(ctx context.Context, userID int, keyword string) ([]*MeetingSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.file_name, t.status, m.summary, t.error_message, m.created_at, m.updated_at
		FROM meetings m
		LEFT JOIN LATERAL (
			SELECT status, error_message FROM meeting_tasks WHERE meeting_id = m.id ORDER BY created_at DESC LIMIT 1
		) t ON true
		WHERE m.user_id = $1
		  AND (
			to_tsvector('russian', COALESCE(m.transcription, '')) @@ plainto_tsquery('russian', $2)
			OR to_tsvector('russian', COALESCE(m.summary, '')) @@ plainto_tsquery('russian', $2)
		  )
		ORDER BY m.created_at DESC
	`, userID, keyword)
	if err != nil {
		return nil, fmt.Errorf("search meetings: %w", err)
	}
	defer rows.Close()

	var meetings []*MeetingSummary
	for rows.Next() {
		m := &MeetingSummary{}
		err := rows.Scan(&m.ID, &m.FileName, &m.Status, &m.Summary, &m.ErrorMessage, &m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

// GetFailedMeetings returns all meetings that failed processing (latest task status = failed).
func (r *MeetingRepo) GetFailedMeetings(ctx context.Context) ([]*Meeting, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.user_id, m.file_name, m.transcription, m.summary,
		       m.gridfs_id, m.created_at, m.updated_at,
		       t.status, t.error_message
		FROM meetings m
		INNER JOIN LATERAL (
			SELECT status, error_message FROM meeting_tasks WHERE meeting_id = m.id ORDER BY created_at DESC LIMIT 1
		) t ON true
		WHERE t.status = $1
	`, string(StatusFailed))
	if err != nil {
		return nil, fmt.Errorf("get failed meetings: %w", err)
	}
	defer rows.Close()

	var meetings []*Meeting
	for rows.Next() {
		m := &Meeting{}
		err := rows.Scan(
			&m.ID, &m.UserID, &m.FileName,
			&m.Transcription, &m.Summary,
			&m.GridFSID, &m.CreatedAt, &m.UpdatedAt,
			&m.Status, &m.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("scan meeting: %w", err)
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

// UpdateMeetingFileName updates the file name for a meeting.
func (r *MeetingRepo) UpdateMeetingFileName(ctx context.Context, meetingID int, fileName string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meetings SET file_name = $1, updated_at = NOW() WHERE id = $2
	`, fileName, meetingID)
	if err != nil {
		return fmt.Errorf("update meeting file name: %w", err)
	}
	return nil
}

// UpdateMeetingGridFSID updates the GridFS file ID for a meeting.
func (r *MeetingRepo) UpdateMeetingGridFSID(ctx context.Context, meetingID int, gridfsID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE meetings SET gridfs_id = $1, updated_at = NOW() WHERE id = $2
	`, gridfsID, meetingID)
	if err != nil {
		return fmt.Errorf("update meeting gridfs id: %w", err)
	}
	return nil
}

// GetMeetingByGridFSID retrieves a meeting by its GridFS file ID with latest task status and error.
func (r *MeetingRepo) GetMeetingByGridFSID(ctx context.Context, gridfsID string) (*Meeting, error) {
	m := &Meeting{}
	err := r.db.QueryRowContext(ctx, `
		SELECT m.id, m.user_id, m.file_name, m.transcription, m.summary,
		       m.gridfs_id, m.created_at, m.updated_at,
		       t.status, t.error_message
		FROM meetings m
		LEFT JOIN LATERAL (
			SELECT status, error_message FROM meeting_tasks WHERE meeting_id = m.id ORDER BY created_at DESC LIMIT 1
		) t ON true
		WHERE m.gridfs_id = $1
	`, gridfsID).Scan(
		&m.ID, &m.UserID, &m.FileName,
		&m.Transcription, &m.Summary,
		&m.GridFSID, &m.CreatedAt, &m.UpdatedAt,
		&m.Status, &m.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("get meeting by gridfs id: %w", err)
	}
	return m, nil
}

// CreateRetryTask creates a new task for retrying a failed meeting.
func (r *MeetingRepo) CreateRetryTask(ctx context.Context, meetingID int) (*MeetingTask, error) {
	var task MeetingTask
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO meeting_tasks (meeting_id, status) VALUES ($1, $2)
		RETURNING id, meeting_id, status, created_at, updated_at
	`, meetingID, string(StatusCreated)).Scan(
		&task.ID, &task.MeetingID, &task.Status,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create retry task: %w", err)
	}
	return &task, nil
}

// DeleteMeeting deletes a meeting and its associated task by ID.
// This uses a transaction to ensure atomicity.
func (r *MeetingRepo) DeleteMeeting(ctx context.Context, meetingID int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Delete associated task first (foreign key constraint)
	_, err = tx.ExecContext(ctx, `DELETE FROM meeting_tasks WHERE meeting_id = $1`, meetingID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("delete meeting task: %w (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("delete meeting task: %w", err)
	}

	// Delete the meeting
	_, err = tx.ExecContext(ctx, `DELETE FROM meetings WHERE id = $1`, meetingID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("delete meeting: %w (rollback: %v)", err, rbErr)
		}
		return fmt.Errorf("delete meeting: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
