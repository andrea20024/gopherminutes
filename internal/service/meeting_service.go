// Package service provides business logic for meeting processing.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andrea20024/goferminutes2/internal/ai"
	"github.com/andrea20024/goferminutes2/internal/logger"
	"github.com/andrea20024/goferminutes2/internal/speech"
	"github.com/andrea20024/goferminutes2/internal/storage"
	"golang.org/x/sync/errgroup"
)

// GridFSStore defines the interface for GridFS storage operations.
type GridFSStore interface {
	UploadFile(ctx context.Context, filePath string) (interface{}, error)
	DownloadToReader(ctx context.Context, fileID string) ([]byte, error)
	DeleteFile(ctx context.Context, fileID string) error
	FileExists(ctx context.Context, fileID string) bool
}

// TaskContext holds data for a background processing task.
type TaskContext struct {
	MeetingID int
	TaskID    int
	FileName  string
	UserID    int
	GridFSID  string
	FileData  []byte
	FileMIME  string
}

// MeetingRepository defines the interface for meeting data access.
// Used to enable mock implementations for unit testing.
type MeetingRepository interface {
	CreateMeetingWithTask(ctx context.Context, userID int, fileName string) (*storage.Meeting, *storage.MeetingTask, error)
	UpdateMeetingStatus(ctx context.Context, meetingID int, status storage.MeetingStatus, errMsg *string) error
	SaveTranscription(ctx context.Context, meetingID int, transcription string) error
	SaveSummary(ctx context.Context, meetingID int, summary string) error
	CompleteMeeting(ctx context.Context, meetingID int) error
	GetMeeting(ctx context.Context, meetingID int) (*storage.Meeting, error)
	ListMeetingsByUser(ctx context.Context, userID int) ([]*storage.MeetingSummary, error)
	SearchMeetingsByKeyword(ctx context.Context, userID int, keyword string) ([]*storage.MeetingSummary, error)
	UpdateMeetingGridFSID(ctx context.Context, meetingID int, gridfsID string) error
	CreateRetryTask(ctx context.Context, meetingID int) (*storage.MeetingTask, error)
}

// MeetingService orchestrates meeting processing: transcription, summarization.
type MeetingService struct {
	mu           sync.Mutex
	stopped      atomic.Bool
	once         sync.Once
	meetingRepo  MeetingRepository
	userRepo     *storage.UserRepo
	speechClient speech.SpeechClient
	llmClient    ai.LLMClient
	gridFS       GridFSStore
	taskQueue    chan *TaskContext
	eg           *errgroup.Group
	egCtx        context.Context
	egCancel     context.CancelFunc
	taskTimeout  time.Duration
}

// NewMeetingService creates a new MeetingService.
// Returns an error if functional options contain invalid values.
func NewMeetingService(
	meetingRepo MeetingRepository,
	userRepo *storage.UserRepo,
	speechClient speech.SpeechClient,
	llmClient ai.LLMClient,
	gridFS GridFSStore,
	opts ...func(*MeetingService) error,
) (*MeetingService, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &MeetingService{
		meetingRepo:  meetingRepo,
		userRepo:     userRepo,
		speechClient: speechClient,
		llmClient:    llmClient,
		gridFS:       gridFS,
		taskQueue:    make(chan *TaskContext, 100),
		egCtx:        ctx,
		egCancel:     cancel,
		taskTimeout:  10 * 60 * time.Second,
	}

	// Initialize errgroup BEFORE applying options so WithWorkers can call SetLimit
	s.eg, s.egCtx = errgroup.WithContext(s.egCtx)
	s.eg.SetLimit(3)

	var applyErr error
	for _, opt := range opts {
		if err := opt(s); err != nil {
			applyErr = err
		}
	}
	if applyErr != nil {
		cancel()
		return nil, fmt.Errorf("apply options: %w", applyErr)
	}

	s.eg.Go(func() error {
		s.consumeLoop()
		return nil
	})

	if logger.Sugar() != nil {
		logger.Sugar().Infow("meeting service started", "queue_capacity", cap(s.taskQueue), "max_workers", 3)
	}

	return s, nil
}

// consumeLoop reads tasks from the queue and dispatches them via errgroup.
func (s *MeetingService) consumeLoop() {
	for task := range s.taskQueue {
		// errgroup.Go blocks when the concurrency limit is reached,
		// replacing the manual semaphore.
		taskCtx := task // capture for closure
		s.eg.Go(func() error {
			s.runTask(taskCtx)
			return nil
		})
	}
}

// runTask handles the full processing pipeline for a meeting.
func (s *MeetingService) runTask(task *TaskContext) {
	ctx, cancel := context.WithTimeout(s.egCtx, s.taskTimeout)
	defer cancel()

	if logger.Sugar() != nil {
		logger.Sugar().Infow("processing meeting", "meeting_id", task.MeetingID)
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		s.failTask(ctx, task, fmt.Sprintf("context error before processing: %v", ctx.Err()))
		return
	}

	// Step 1: Update status to processing
	err := s.meetingRepo.UpdateMeetingStatus(ctx, task.MeetingID, storage.StatusProcessing, nil)
	if err != nil {
		s.failTask(ctx, task, fmt.Sprintf("update status to processing: %v", err))
		return
	}

	// Step 2: Transcription
	transcript, err := s.speechClient.Recognize(ctx, task.FileData, task.FileMIME)
	if err != nil {
		// Check if it's a speech client error to provide better context
		if IsSpeechClientError(err) {
			var sce *SpeechClientError
			if errors.As(err, &sce) {
				s.failTask(ctx, task, fmt.Sprintf("speech recognition failed (transient=%v): %s", sce.Transient, sce.Message))
			} else {
				s.failTask(ctx, task, fmt.Sprintf("speech recognition failed: %v", err))
			}
		} else {
			s.failTask(ctx, task, fmt.Sprintf("speech recognition failed: %v", err))
		}
		return
	}

	if err := s.meetingRepo.SaveTranscription(ctx, task.MeetingID, transcript); err != nil {
		s.failTask(ctx, task, fmt.Sprintf("save transcription: %v", err))
		return
	}

	if err := s.meetingRepo.UpdateMeetingStatus(ctx, task.MeetingID, storage.StatusTranscribed, nil); err != nil {
		s.failTask(ctx, task, fmt.Sprintf("update status to transcribed: %v", err))
		return
	}

	if logger.Sugar() != nil {
		logger.Sugar().Infow("transcription saved", "meeting_id", task.MeetingID)
	}

	// Step 3: Summarization
	summary, err := s.llmClient.GetSummary(ctx, transcript)
	if err != nil {
		// Check if it's an LLM client error to provide better context
		if IsLLMClientError(err) {
			var lle *LLMClientError
			if errors.As(err, &lle) {
				s.failTask(ctx, task, fmt.Sprintf("LLM summarization failed (transient=%v): %s", lle.Transient, lle.Message))
			} else {
				s.failTask(ctx, task, fmt.Sprintf("LLM summarization failed: %v", err))
			}
		} else {
			s.failTask(ctx, task, fmt.Sprintf("LLM summarization failed: %v", err))
		}
		return
	}

	if err := s.meetingRepo.SaveSummary(ctx, task.MeetingID, summary); err != nil {
		s.failTask(ctx, task, fmt.Sprintf("save summary: %v", err))
		return
	}

	if err := s.meetingRepo.UpdateMeetingStatus(ctx, task.MeetingID, storage.StatusSummarized, nil); err != nil {
		s.failTask(ctx, task, fmt.Sprintf("update status to summarized: %v", err))
		return
	}

	// Step 4: Complete
	if err := s.meetingRepo.CompleteMeeting(ctx, task.MeetingID); err != nil {
		s.failTask(ctx, task, fmt.Sprintf("complete meeting: %v", err))
		return
	}

	if logger.Sugar() != nil {
		logger.Sugar().Infow("meeting processing completed", "meeting_id", task.MeetingID)
	}
}

// failTask marks a meeting as failed with an error message.
func (s *MeetingService) failTask(ctx context.Context, task *TaskContext, errMsg string) {
	if logger.Sugar() != nil {
		logger.Sugar().Errorw("meeting processing failed",
			"meeting_id", task.MeetingID,
			"error", errMsg,
		)
	}

	errStr := errMsg
	dCtx, dCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dCancel()
	if err := s.meetingRepo.UpdateMeetingStatus(dCtx, task.MeetingID, storage.StatusFailed, &errStr); err != nil {
		if logger.Sugar() != nil {
			logger.Sugar().Errorw("failed to persist failure status",
				"meeting_id", task.MeetingID,
				"error", err,
			)
		}
	}
}

// StartProcessing creates a meeting and enqueues it for processing.
func (s *MeetingService) StartProcessing(ctx context.Context, userID int, filePath string, fileData []byte, fileMIME string) (*storage.Meeting, *storage.MeetingTask, error) {
	fileName := filepath.Base(filePath)
	meeting, task, err := s.meetingRepo.CreateMeetingWithTask(ctx, userID, fileName)
	if err != nil {
		return nil, nil, fmt.Errorf("create meeting with task: %w", err)
	}

	// Upload file to GridFS if available
	var gridfsID string
	if s.gridFS != nil {
		fileID, err := s.gridFS.UploadFile(ctx, filePath)
		if err != nil {
			logger.Sugar().Errorw("failed to upload file to GridFS",
				"error", err,
				"path", filePath,
				"meeting_id", meeting.ID,
			)
		} else {
			gridfsID = fmt.Sprintf("%v", fileID)
			if err := s.meetingRepo.UpdateMeetingGridFSID(ctx, meeting.ID, gridfsID); err != nil {
				logger.Sugar().Errorw("failed to update GridFS ID", "error", err)
			}
		}
	}

	if logger.Sugar() != nil {
		logger.Sugar().Infow("meeting created and enqueued",
			"meeting_id", meeting.ID,
			"user_id", userID,
			"file_name", fileName,
			"gridfs_id", gridfsID,
			"file_size", len(fileData),
			"mime", fileMIME,
		)
	}

	taskCtx := &TaskContext{
		MeetingID: meeting.ID,
		TaskID:    task.ID,
		FileName:  fileName,
		UserID:    userID,
		GridFSID:  gridfsID,
		FileData:  fileData,
		FileMIME:  fileMIME,
	}

	// Check if service is stopped to prevent panic on closed channel
	if s.stopped.Load() {
		return nil, nil, ErrServiceShuttingDown
	}
	s.mu.Lock()
	select {
	case s.taskQueue <- taskCtx:
		s.mu.Unlock()
		return meeting, task, nil
	case <-s.egCtx.Done():
		s.mu.Unlock()
		return nil, nil, ErrServiceShuttingDown
	}
}

// GetMeeting retrieves a meeting by ID with user ownership check.
func (s *MeetingService) GetMeeting(ctx context.Context, meetingID, userID int) (*storage.Meeting, error) {
	meeting, err := s.meetingRepo.GetMeeting(ctx, meetingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, WrapMeetingNotFound(err)
		}
		return nil, fmt.Errorf("get meeting: %w", err)
	}

	if meeting.UserID != userID {
		return nil, WrapAccessDenied(fmt.Errorf("meeting %d does not belong to user %d", meetingID, userID))
	}

	return meeting, nil
}

// ListMeetings returns all meetings for a user.
func (s *MeetingService) ListMeetings(ctx context.Context, userID int) ([]*storage.MeetingSummary, error) {
	return s.meetingRepo.ListMeetingsByUser(ctx, userID)
}

// SearchMeetings searches meetings by keyword for a specific user.
func (s *MeetingService) SearchMeetings(ctx context.Context, userID int, keyword string) ([]*storage.MeetingSummary, error) {
	return s.meetingRepo.SearchMeetingsByKeyword(ctx, userID, keyword)
}

// Chat asks a question about the meeting materials.
func (s *MeetingService) Chat(ctx context.Context, userID int, question string, meetingID *int) (string, error) {
	var contextText string

	if meetingID != nil {
		meeting, err := s.GetMeeting(ctx, *meetingID, userID)
		if err != nil {
			return "", fmt.Errorf("get meeting for chat: %w", err)
		}
		if meeting.Transcription != nil {
			contextText = *meeting.Transcription
		}
	}

	if contextText == "" {
		return s.llmClient.Ask(ctx, question, "")
	}

	return s.llmClient.Ask(ctx, question, contextText)
}

// RetryProcessing retries a failed meeting by creating a new task.
func (s *MeetingService) RetryProcessing(ctx context.Context, meetingID, userID int) (*storage.Meeting, error) {
	meeting, err := s.GetMeeting(ctx, meetingID, userID)
	if err != nil {
		return nil, fmt.Errorf("get meeting for retry: %w", err)
	}

	if meeting.Status != storage.StatusFailed {
		return nil, fmt.Errorf("%w: current status is %s", ErrRetryOnNonFailedMeeting, meeting.Status)
	}

	// Check if file exists in GridFS
	var fileData []byte
	var fileMIME string
	if s.gridFS != nil && meeting.GridFSID != nil {
		data, err := s.gridFS.DownloadToReader(ctx, *meeting.GridFSID)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGridFSFileNotFound, err)
		}
		fileData = data
		fileMIME = GetMIMEType(filepath.Ext(meeting.FileName))
	}

	// Create a new task for retry
	_, err = s.meetingRepo.CreateRetryTask(ctx, meetingID)
	if err != nil {
		return nil, fmt.Errorf("create retry task: %w", err)
	}

	taskCtx := &TaskContext{
		MeetingID: meetingID,
		FileName:  meeting.FileName,
		UserID:    userID,
		FileData:  fileData,
		FileMIME:  fileMIME,
	}

	// Check if service is stopped to prevent panic on closed channel
	if s.stopped.Load() {
		return nil, ErrServiceShuttingDown
	}
	s.mu.Lock()
	select {
	case s.taskQueue <- taskCtx:
		s.mu.Unlock()
		return meeting, nil
	case <-s.egCtx.Done():
		s.mu.Unlock()
		return nil, ErrServiceShuttingDown
	}
}

// Stop gracefully shuts down the service.
// Safe to call multiple times — only executes once.
func (s *MeetingService) Stop() {
	s.once.Do(func() {
		if logger.Sugar() != nil {
			logger.Sugar().Infow("stopping meeting service")
		}

		// Close the queue to stop consumeLoop
		// Mark as stopped before closing the queue to prevent new sends.
		s.stopped.Store(true)
		close(s.taskQueue)

		// Wait for all goroutines to finish processing remaining tasks
		_ = s.eg.Wait()

		// Cancel context after all tasks complete
		s.egCancel()

		if logger.Sugar() != nil {
			logger.Sugar().Infow("meeting service stopped")
		}
	})
}

// WithWorkers sets the max number of concurrent processing workers.
func WithWorkers(n int) func(*MeetingService) error {
	return func(s *MeetingService) error {
		if n <= 0 {
			return fmt.Errorf("workers must be > 0, got %d", n)
		}
		s.eg.SetLimit(n)
		return nil
	}
}

// WithQueueCapacity sets the task queue capacity.
func WithQueueCapacity(n int) func(*MeetingService) error {
	return func(s *MeetingService) error {
		if n <= 0 {
			return fmt.Errorf("queue capacity must be > 0, got %d", n)
		}
		s.taskQueue = make(chan *TaskContext, n)
		return nil
	}
}

// WithTaskTimeout sets the timeout for each processing task.
func WithTaskTimeout(d time.Duration) func(*MeetingService) error {
	return func(s *MeetingService) error {
		if d <= 0 {
			return fmt.Errorf("task timeout must be > 0, got %v", d)
		}
		s.taskTimeout = d
		return nil
	}
}
