package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andrea20024/goferminutes2/internal/storage"
)

// MockMeetingRepo is a thread-safe in-memory mock for unit tests.
type MockMeetingRepo struct {
	mu             sync.RWMutex
	meetings       map[int]*storage.Meeting
	tasks          map[int]*storage.MeetingTask
	nextID         int
	shouldFailNext bool
	failErr        error
}

// NewMockMeetingRepo creates a new MockMeetingRepo.
func NewMockMeetingRepo() *MockMeetingRepo {
	return &MockMeetingRepo{
		meetings: make(map[int]*storage.Meeting),
		tasks:    make(map[int]*storage.MeetingTask),
		nextID:   1,
	}
}

// SetFailNext configures the mock to fail on the next operation.
func (m *MockMeetingRepo) SetFailNext(fail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFailNext = fail
	m.failErr = err
}

// latestTaskForMeeting returns the task with the highest ID for a given meeting.
// It assumes the lock is already held by the caller.
func (m *MockMeetingRepo) latestTaskForMeeting(meetingID int) *storage.MeetingTask {
	var latest *storage.MeetingTask
	var latestID int
	for _, task := range m.tasks {
		if task.MeetingID == meetingID && task.ID > latestID {
			latestID = task.ID
			latest = task
		}
	}
	return latest
}

// GetMeeting returns the meeting by ID.
func (m *MockMeetingRepo) GetMeeting(ctx context.Context, meetingID int) (*storage.Meeting, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	meeting, exists := m.meetings[meetingID]
	if !exists {
		return nil, fmt.Errorf("meeting not found: %d", meetingID)
	}
	// Return a copy to avoid race conditions
	cpy := *meeting
	if task := m.latestTaskForMeeting(meetingID); task != nil {
		cpy.Status = task.Status
	}
	return &cpy, nil
}

// CreateMeetingWithTask creates a meeting and task in a simulated transaction.
func (m *MockMeetingRepo) CreateMeetingWithTask(ctx context.Context, userID int, fileName string) (*storage.Meeting, *storage.MeetingTask, error) {
	m.mu.Lock()
	if m.shouldFailNext {
		err := m.failErr
		m.shouldFailNext = false
		m.mu.Unlock()
		return nil, nil, err
	}

	id := m.nextID
	m.nextID++

	now := time.Now()

	meeting := &storage.Meeting{
		ID:        id,
		UserID:    userID,
		FileName:  fileName,
		Status:    storage.StatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}

	task := &storage.MeetingTask{
		ID:        id,
		MeetingID: id,
		Status:    storage.StatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.meetings[id] = meeting
	m.tasks[id] = task
	m.mu.Unlock()

	return meeting, task, nil
}

// UpdateMeetingStatus updates the latest task status for a meeting.
func (m *MockMeetingRepo) UpdateMeetingStatus(ctx context.Context, meetingID int, status storage.MeetingStatus, errMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.latestTaskForMeeting(meetingID)
	if task == nil {
		return fmt.Errorf("task not found for meeting: %d", meetingID)
	}

	task.Status = status
	task.UpdatedAt = time.Now()
	if errMsg != nil {
		task.ErrorMessage = errMsg
	}
	return nil
}

// SaveTranscription saves the transcription for a meeting and updates task status.
func (m *MockMeetingRepo) SaveTranscription(ctx context.Context, meetingID int, transcription string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meeting, exists := m.meetings[meetingID]
	if !exists {
		return fmt.Errorf("meeting not found: %d", meetingID)
	}

	meeting.Transcription = &transcription
	meeting.UpdatedAt = time.Now()

	if task := m.latestTaskForMeeting(meetingID); task != nil {
		task.Status = storage.StatusTranscribed
		task.UpdatedAt = time.Now()
	}
	return nil
}

// SaveSummary saves the summary for a meeting and updates task status.
func (m *MockMeetingRepo) SaveSummary(ctx context.Context, meetingID int, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meeting, exists := m.meetings[meetingID]
	if !exists {
		return fmt.Errorf("meeting not found: %d", meetingID)
	}

	meeting.Summary = &summary
	meeting.UpdatedAt = time.Now()

	if task := m.latestTaskForMeeting(meetingID); task != nil {
		task.Status = storage.StatusSummarized
		task.UpdatedAt = time.Now()
	}
	return nil
}

// CompleteMeeting marks the latest task for a meeting as completed.
func (m *MockMeetingRepo) CompleteMeeting(ctx context.Context, meetingID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := m.latestTaskForMeeting(meetingID)
	if task == nil {
		return fmt.Errorf("task not found for meeting: %d", meetingID)
	}

	task.Status = storage.StatusCompleted
	task.UpdatedAt = time.Now()
	return nil
}

// CreateRetryTask creates a new task for retrying a failed meeting.
func (m *MockMeetingRepo) CreateRetryTask(ctx context.Context, meetingID int) (*storage.MeetingTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	now := time.Now()

	task := &storage.MeetingTask{
		ID:        id,
		MeetingID: meetingID,
		Status:    storage.StatusCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.tasks[id] = task
	return task, nil
}

// ListMeetingsByUser returns all meetings for a user with latest task status.
func (m *MockMeetingRepo) ListMeetingsByUser(ctx context.Context, userID int) ([]*storage.MeetingSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*storage.MeetingSummary
	for _, meeting := range m.meetings {
		if meeting.UserID == userID {
			task := m.latestTaskForMeeting(meeting.ID)
			summary := &storage.MeetingSummary{
				ID:           meeting.ID,
				FileName:     meeting.FileName,
				Status:       task.Status,
				Summary:      meeting.Summary,
				ErrorMessage: meeting.ErrorMessage,
				CreatedAt:    meeting.CreatedAt,
				UpdatedAt:    meeting.UpdatedAt,
			}
			results = append(results, summary)
		}
	}
	// Sort by created_at DESC (simple bubble sort for small datasets)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].CreatedAt.After(results[i].CreatedAt) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	return results, nil
}

// SearchMeetingsByKeyword searches meetings by keyword.
func (m *MockMeetingRepo) SearchMeetingsByKeyword(ctx context.Context, userID int, keyword string) ([]*storage.MeetingSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*storage.MeetingSummary
	lowerKeyword := strings.ToLower(keyword)

	for _, meeting := range m.meetings {
		if meeting.UserID != userID {
			continue
		}

		match := false
		if meeting.Transcription != nil && strings.Contains(strings.ToLower(*meeting.Transcription), lowerKeyword) {
			match = true
		}
		if meeting.Summary != nil && strings.Contains(strings.ToLower(*meeting.Summary), lowerKeyword) {
			match = true
		}

		if match {
			task := m.latestTaskForMeeting(meeting.ID)
			summary := &storage.MeetingSummary{
				ID:           meeting.ID,
				FileName:     meeting.FileName,
				Status:       task.Status,
				Summary:      meeting.Summary,
				ErrorMessage: meeting.ErrorMessage,
				CreatedAt:    meeting.CreatedAt,
				UpdatedAt:    meeting.UpdatedAt,
			}
			results = append(results, summary)
		}
	}
	return results, nil
}

// UpdateMeetingGridFSID updates the GridFS ID for a meeting.
func (m *MockMeetingRepo) UpdateMeetingGridFSID(ctx context.Context, meetingID int, gridfsID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meeting, exists := m.meetings[meetingID]
	if !exists {
		return fmt.Errorf("meeting not found: %d", meetingID)
	}

	meeting.GridFSID = &gridfsID
	meeting.UpdatedAt = time.Now()
	m.meetings[meetingID] = meeting
	return nil
}

// GetMeetingByID returns a meeting by ID (for internal use).
func (m *MockMeetingRepo) GetMeetingByID(meetingID int) *storage.Meeting {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if meeting, exists := m.meetings[meetingID]; exists {
		cpy := *meeting
		if task := m.latestTaskForMeeting(meetingID); task != nil {
			cpy.Status = task.Status
			cpy.ErrorMessage = task.ErrorMessage
		}
		return &cpy
	}
	return nil
}

// GetTaskByID returns the latest task for a meeting by meeting ID.
func (m *MockMeetingRepo) GetTaskByID(meetingID int) *storage.MeetingTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task := m.latestTaskForMeeting(meetingID)
	if task == nil {
		return nil
	}
	cpy := *task
	return &cpy
}

// AllMeetings returns all meetings (for testing).
func (m *MockMeetingRepo) AllMeetings() map[int]*storage.Meeting {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cpy := make(map[int]*storage.Meeting, len(m.meetings))
	for k, v := range m.meetings {
		c := *v
		if task := m.latestTaskForMeeting(k); task != nil {
			c.Status = task.Status
		}
		cpy[k] = &c
	}
	return cpy
}
