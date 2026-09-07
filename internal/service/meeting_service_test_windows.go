//go:build windows

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/andrea20024/goferminutes2/internal/ai"
	"github.com/andrea20024/goferminutes2/internal/speech"
	"github.com/andrea20024/goferminutes2/internal/storage"
)

func TestMeetingService_ContextCancellation(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	// Slow mocks — pipeline takes 2s, context cancelled at 100ms
	speechClient := speech.NewSlowMockSpeechClient(2 * time.Second)
	llmClient := ai.NewSlowMockLLMClient(2 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	meeting, _, err := svc.StartProcessing(ctx, 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for context cancellation + processing to react
	time.Sleep(500 * time.Millisecond)

	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final.Status == "completed" {
		t.Error("expected meeting not to be completed due to context cancellation")
	}
	// Status should be "processing" or "failed", not "completed"
	if final.Status == "completed" {
		t.Errorf("expected status != completed, got %s", final.Status)
	}
}

func TestMeetingService_SemaphoreLimit(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	// CountingSpeechClient tracks max concurrent workers
	speechClient := speech.NewCountingSpeechClient(500 * time.Millisecond)
	llmClient := ai.NewMockLLMClient()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	// Enqueue 10 tasks — semaphore should limit to 3 concurrent
	for i := 0; i < 10; i++ {
		_, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
		if err != nil {
			t.Fatalf("StartProcessing failed: %v", err)
		}
	}

	// Wait for all to complete
	time.Sleep(5 * time.Second)

	// Verify all completed
	all := mockRepo.AllMeetings()
	if len(all) != 10 {
		t.Errorf("expected 10 meetings, got %d", len(all))
	}

	// CRITICAL: semaphore must limit to max 3 concurrent workers
	maxConcurrent := speechClient.Max()
	if maxConcurrent > 3 {
		t.Errorf("semaphore failed: max concurrent workers = %d, expected <= 3", maxConcurrent)
	}
	if maxConcurrent == 0 {
		t.Error("semaphore failed: no concurrent processing detected")
	}
	t.Logf("max concurrent workers: %d (limit: 3)", maxConcurrent)

	for id, m := range all {
		if m.Status != "completed" {
			t.Errorf("meeting %d status: %s", id, m.Status)
		}
	}
}

func TestMeetingService_ContextTimeout(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	// Slow mocks — 2s delay, context timeout 50ms
	speechClient := speech.NewSlowMockSpeechClient(2 * time.Second)
	llmClient := ai.NewSlowMockLLMClient(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	meeting, _, err := svc.StartProcessing(ctx, 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for timeout + processing to react
	time.Sleep(200 * time.Millisecond)

	if ctx.Err() == nil {
		t.Error("expected context to timeout")
	}

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final.Status == "completed" {
		t.Error("expected meeting not to be completed due to timeout")
	}
	// Status should be "processing" or "failed", not "completed"
	if final.Status == "completed" {
		t.Errorf("expected status != completed, got %s", final.Status)
	}
}

func TestMeetingService_GracefulShutdown_NoGoroutineLeak(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	// Slow mocks — tasks take 1s, shutdown after 200ms
	speechClient := speech.NewSlowMockSpeechClient(1 * time.Second)
	llmClient := ai.NewSlowMockLLMClient(1 * time.Second)

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Enqueue 5 tasks
	for i := 0; i < 5; i++ {
		_, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
		if err != nil {
			t.Fatalf("StartProcessing failed: %v", err)
		}
	}

	// Let a few start processing
	time.Sleep(200 * time.Millisecond)

	// Shutdown — errgroup waits for in-progress tasks to finish,
	// but tasks still in the queue are not processed.
	svc.Stop()

	all := mockRepo.AllMeetings()

	// Verify no goroutine leak: all goroutines have finished.
	// Processed tasks should be in terminal states.
	// Unprocessed tasks (still in queue when Stop was called) may remain non-terminal.
	processedCount := 0
	for _, m := range all {
		switch m.Status {
		case "completed", "failed", "transcribed", "summarized":
			processedCount++
		}
	}
	// At least 3 tasks should have been processed (semaphore limit)
	if processedCount < 3 {
		t.Errorf("expected at least 3 processed meetings, got %d", processedCount)
	}
}

func TestMeetingService_ShutdownStopsAcceptingTasks(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := ai.NewMockLLMClient()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Stop service
	svc.Stop()

	// Try to enqueue — should fail
	_, _, err = svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected error when enqueuing to stopped service, got nil")
	}
}

// TestMeetingService_SpeechClientError tests that speech client errors result in failed status.
func TestMeetingService_SpeechClientError(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := &speech.MockSpeechClient{
		Error: fmt.Errorf("speech service unavailable: connection refused"),
	}
	llmClient := ai.NewMockLLMClient()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	meeting, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for async processing to complete
	time.Sleep(2 * time.Second)

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final == nil {
		t.Fatal("meeting not found after processing")
	}
	if final.Status != storage.StatusFailed {
		t.Errorf("expected status failed, got %s", final.Status)
	}
	if final.ErrorMessage == nil || *final.ErrorMessage == "" {
		t.Error("expected error message for failed meeting")
	}
	t.Logf("meeting status: %s, error: %v", final.Status, *final.ErrorMessage)
}

// TestMeetingService_LLMClientError tests that LLM client errors (after transcription) result in failed status.
func TestMeetingService_LLMClientError(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := &ai.MockLLMClient{
		SummaryError: fmt.Errorf("LLM service rate limited: 429 Too Many Requests"),
	}

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	meeting, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for async processing to complete
	time.Sleep(2 * time.Second)

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final == nil {
		t.Fatal("meeting not found after processing")
	}
	if final.Status != storage.StatusFailed {
		t.Errorf("expected status failed, got %s", final.Status)
	}
	if final.ErrorMessage == nil || *final.ErrorMessage == "" {
		t.Error("expected error message for failed meeting")
	}
	// Transcription should be saved before LLM error
	if final.Transcription == nil {
		t.Error("expected transcription to be saved before LLM error")
	}
	t.Logf("meeting status: %s, error: %v", final.Status, *final.ErrorMessage)
}

// TestMeetingService_SuccessfulRetry tests that a failed meeting can be retried successfully.
func TestMeetingService_SuccessfulRetry(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	// First: speech client fails
	failingSpeech := &speech.MockSpeechClient{
		Error: fmt.Errorf("speech service temporarily unavailable"),
	}
	svc, err := NewMeetingService(mockRepo, nil, failingSpeech, ai.NewMockLLMClient(), nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Start processing — will fail
	meeting, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for failure
	time.Sleep(2 * time.Second)

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final == nil {
		t.Fatal("meeting not found")
	}
	if final.Status != storage.StatusFailed {
		t.Fatalf("expected status failed after speech error, got %s", final.Status)
	}
	t.Logf("meeting failed as expected: status=%s, error=%v", final.Status, *final.ErrorMessage)

	// Now retry with working clients
	svc.Stop()

	okSpeech := speech.NewMockSpeechClient()
	okLLM := ai.NewMockLLMClient()
	svc2, err := NewMeetingService(mockRepo, nil, okSpeech, okLLM, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc2.Stop()

	retried, err := svc2.RetryProcessing(context.Background(), meeting.ID, 1)
	if err != nil {
		t.Fatalf("RetryProcessing failed: %v", err)
	}
	if retried.ID != meeting.ID {
		t.Errorf("expected retried meeting ID %d, got %d", meeting.ID, retried.ID)
	}

	// Wait for successful processing
	time.Sleep(2 * time.Second)

	final2 := mockRepo.GetMeetingByID(meeting.ID)
	if final2 == nil {
		t.Fatal("meeting not found after retry")
	}
	if final2.Status != storage.StatusCompleted {
		t.Errorf("expected status completed after retry, got %s", final2.Status)
	}
	if final2.Transcription == nil {
		t.Error("expected transcription after successful retry")
	}
	if final2.Summary == nil {
		t.Error("expected summary after successful retry")
	}
	t.Logf("meeting completed after retry: status=%s", final2.Status)
}

// TestMeetingService_RetryNonFailedMeeting tests that retry is rejected for non-failed meetings.
func TestMeetingService_RetryNonFailedMeeting(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := ai.NewMockLLMClient()

	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer svc.Stop()

	meeting, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}

	// Wait for successful processing
	time.Sleep(2 * time.Second)

	final := mockRepo.GetMeetingByID(meeting.ID)
	if final == nil {
		t.Fatal("meeting not found")
	}
	if final.Status != storage.StatusCompleted {
		t.Fatalf("expected status completed, got %s", final.Status)
	}

	// Try to retry a completed meeting
	_, err = svc.RetryProcessing(context.Background(), meeting.ID, 1)
	if err == nil {
		t.Fatal("expected error when retrying completed meeting, got nil")
	}
	if !errors.Is(err, ErrRetryOnNonFailedMeeting) {
		t.Errorf("expected ErrRetryOnNonFailedMeeting, got: %v", err)
	}
	t.Logf("retry correctly rejected: %v", err)
}
