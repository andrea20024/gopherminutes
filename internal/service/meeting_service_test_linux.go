//go:build linux && unit

package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/andrea20024/goferminutes2/internal/ai"
	"github.com/andrea20024/goferminutes2/internal/speech"
	"github.com/andrea20024/goferminutes2/internal/storage"
)

func setupTestService(t *testing.T) (*MeetingService, *MockMeetingRepo, *speech.MockSpeechClient, *ai.MockLLMClient) {
	t.Helper()
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := ai.NewMockLLMClient()
	svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	t.Cleanup(func() { svc.Stop() })
	return svc, mockRepo, speechClient, llmClient
}

func TestMeetingService_StartProcessing(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		svc, mockRepo, _, _ := setupTestService(t)

		meeting, task, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("audio data"), "audio/mpeg")
		if err != nil {
			t.Fatalf("StartProcessing failed: %v", err)
		}
		if meeting == nil || task == nil {
			t.Fatal("expected meeting and task to be returned")
		}
		if meeting.UserID != 1 {
			t.Errorf("expected userID 1, got %d", meeting.UserID)
		}
		if meeting.Status != storage.StatusCreated {
			t.Errorf("expected status created, got %s", meeting.Status)
		}

		synctest.Sleep(2 * time.Second)

		final := mockRepo.GetMeetingByID(meeting.ID)
		if final == nil {
			t.Fatal("meeting not found after processing")
		}
		if final.Status != storage.StatusCompleted {
			t.Errorf("expected status completed, got %s", final.Status)
		}
		if final.Transcription == nil {
			t.Error("expected transcription to be set")
		}
		if final.Summary == nil {
			t.Error("expected summary to be set")
		}
	})
}

func TestMeetingService_ContextCancellation(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		mockRepo := NewMockMeetingRepo()
		speechClient := speech.NewSlowMockSpeechClient(2 * time.Second)
		llmClient := ai.NewSlowMockLLMClient(2 * time.Second)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			synctest.Sleep(100 * time.Millisecond)
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

		synctest.Sleep(500 * time.Millisecond)

		if ctx.Err() == nil {
			t.Error("expected context to be cancelled")
		}

		final := mockRepo.GetMeetingByID(meeting.ID)
		if final.Status == storage.StatusCompleted {
			t.Errorf("expected meeting not to be completed due to context cancellation, got %s", final.Status)
		}
	})
}

func TestMeetingService_SemaphoreLimit(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		mockRepo := NewMockMeetingRepo()
		speechClient := speech.NewCountingSpeechClient(500 * time.Millisecond)
		llmClient := ai.NewMockLLMClient()

		svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}
		defer svc.Stop()

		for i := 0; i < 10; i++ {
			_, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
			if err != nil {
				t.Fatalf("StartProcessing failed: %v", err)
			}
		}

		synctest.Sleep(5 * time.Second)

		all := mockRepo.AllMeetings()
		if len(all) != 10 {
			t.Errorf("expected 10 meetings, got %d", len(all))
		}

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
	})
}

func TestMeetingService_ContextTimeout(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		mockRepo := NewMockMeetingRepo()
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

		synctest.Sleep(200 * time.Millisecond)

		if ctx.Err() == nil {
			t.Error("expected context to timeout")
		}

		final := mockRepo.GetMeetingByID(meeting.ID)
		if final.Status == storage.StatusCompleted {
			t.Errorf("expected meeting not to be completed due to timeout, got %s", final.Status)
		}
	})
}

func TestMeetingService_GracefulShutdown_NoGoroutineLeak(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		mockRepo := NewMockMeetingRepo()
		speechClient := speech.NewSlowMockSpeechClient(1 * time.Second)
		llmClient := ai.NewSlowMockLLMClient(1 * time.Second)

		svc, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		for i := 0; i < 5; i++ {
			_, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
			if err != nil {
				t.Fatalf("StartProcessing failed: %v", err)
			}
		}

		synctest.Sleep(200 * time.Millisecond)

		svc.Stop()

		all := mockRepo.AllMeetings()

		processedCount := 0
		for _, m := range all {
			switch m.Status {
			case "completed", "failed", "transcribed", "summarized":
				processedCount++
			}
		}
		if processedCount < 3 {
			t.Errorf("expected at least 3 processed meetings, got %d", processedCount)
		}
	})
}

func TestMeetingService_SpeechClientError(t *testing.T) {
	synctest.Run(func(t *testing.T) {
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

		synctest.Sleep(2 * time.Second)

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
	})
}

func TestMeetingService_LLMClientError(t *testing.T) {
	synctest.Run(func(t *testing.T) {
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

		synctest.Sleep(2 * time.Second)

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
		if final.Transcription == nil {
			t.Error("expected transcription to be saved before LLM error")
		}
		t.Logf("meeting status: %s, error: %v", final.Status, *final.ErrorMessage)
	})
}

func TestMeetingService_SuccessfulRetry(t *testing.T) {
	synctest.Run(func(t *testing.T) {
		mockRepo := NewMockMeetingRepo()

		failingSpeech := &speech.MockSpeechClient{
			Error: fmt.Errorf("speech service temporarily unavailable"),
		}
		svc, err := NewMeetingService(mockRepo, nil, failingSpeech, ai.NewMockLLMClient(), nil)
		if err != nil {
			t.Fatalf("failed to create service: %v", err)
		}

		meeting, _, err := svc.StartProcessing(context.Background(), 1, "test.mp3", []byte("data"), "audio/mpeg")
		if err != nil {
			t.Fatalf("StartProcessing failed: %v", err)
		}

		synctest.Sleep(2 * time.Second)

		final := mockRepo.GetMeetingByID(meeting.ID)
		if final == nil {
			t.Fatal("meeting not found")
		}
		if final.Status != storage.StatusFailed {
			t.Fatalf("expected status failed after speech error, got %s", final.Status)
		}
		t.Logf("meeting failed as expected: status=%s, error=%v", final.Status, *final.ErrorMessage)

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

		synctest.Sleep(2 * time.Second)

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
	})
}

func TestMeetingService_RetryNonFailedMeeting(t *testing.T) {
	synctest.Run(func(t *testing.T) {
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

		synctest.Sleep(2 * time.Second)

		final := mockRepo.GetMeetingByID(meeting.ID)
		if final == nil {
			t.Fatal("meeting not found")
		}
		if final.Status != storage.StatusCompleted {
			t.Fatalf("expected status completed, got %s", final.Status)
		}

		_, err = svc.RetryProcessing(context.Background(), meeting.ID, 1)
		if err == nil {
			t.Fatal("expected error when retrying completed meeting, got nil")
		}
		if !errors.Is(err, ErrRetryOnNonFailedMeeting) {
			t.Errorf("expected ErrRetryOnNonFailedMeeting, got: %v", err)
		}
		t.Logf("retry correctly rejected: %v", err)
	})
}
