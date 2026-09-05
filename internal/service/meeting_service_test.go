//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
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

	time.Sleep(2 * time.Second)

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
}

func TestMeetingService_GetMeeting_UserRestriction(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	meeting, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	if err != nil {
		t.Fatalf("CreateMeetingWithTask failed: %v", err)
	}

	_, err = svc.GetMeeting(context.Background(), meeting.ID, 2)
	if err == nil {
		t.Fatal("expected access denied error, got nil")
	}
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied, got: %v", err)
	}

	result, err := svc.GetMeeting(context.Background(), meeting.ID, 1)
	if err != nil {
		t.Fatalf("GetMeeting for owner failed: %v", err)
	}
	if result.ID != meeting.ID {
		t.Errorf("expected meeting ID %d, got %d", meeting.ID, result.ID)
	}
}

func TestMeetingService_GetMeeting_NotFound(t *testing.T) {
	svc, _, _, _ := setupTestService(t)

	_, err := svc.GetMeeting(context.Background(), 9999, 1)
	if err == nil {
		t.Fatal("expected error for non-existent meeting, got nil")
	}
}

func TestMeetingService_ListMeetings(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	for i := 1; i <= 3; i++ {
		_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, fmt.Sprintf("meeting_%d.mp3", i))
		if err != nil {
			t.Fatalf("CreateMeetingWithTask failed: %v", err)
		}
	}

	_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 2, "meeting_user2.mp3")
	if err != nil {
		t.Fatalf("CreateMeetingWithTask failed: %v", err)
	}

	meetings, err := svc.ListMeetings(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListMeetings failed: %v", err)
	}
	if len(meetings) != 3 {
		t.Errorf("expected 3 meetings, got %d", len(meetings))
	}

	meetings2, err := svc.ListMeetings(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListMeetings failed: %v", err)
	}
	if len(meetings2) != 1 {
		t.Errorf("expected 1 meeting, got %d", len(meetings2))
	}
}

func TestMeetingService_SearchMeetings(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	meeting, _, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	transcription := "Обсуждался план проекта и распределение задач между участниками команды"
	mockRepo.SaveTranscription(context.Background(), meeting.ID, transcription)

	results, err := svc.SearchMeetings(context.Background(), 1, "проект")
	if err != nil {
		t.Fatalf("SearchMeetings failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != meeting.ID {
		t.Errorf("expected meeting ID %d, got %d", meeting.ID, results[0].ID)
	}

	results2, err := svc.SearchMeetings(context.Background(), 1, "несуществующее")
	if err != nil {
		t.Fatalf("SearchMeetings failed: %v", err)
	}
	if len(results2) != 0 {
		t.Errorf("expected 0 results, got %d", len(results2))
	}
}

func TestMeetingService_ChatWithContext(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	meeting, _, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	transcription := "На встрече обсуждался план проекта на Q4"
	mockRepo.SaveTranscription(context.Background(), meeting.ID, transcription)

	question := "Что обсуждалось на встрече?"
	answer, err := svc.Chat(context.Background(), 1, question, &meeting.ID)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if answer == "" {
		t.Error("expected non-empty answer")
	}
}

func TestMeetingService_ChatWithoutContext(t *testing.T) {
	svc, _, _, _ := setupTestService(t)

	question := "Какой сегодня день?"
	answer, err := svc.Chat(context.Background(), 1, question, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if answer == "" {
		t.Error("expected non-empty answer")
	}
}

func TestMeetingService_ChatNoTranscription(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	meeting, _, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")

	question := "Вопрос"
	answer, err := svc.Chat(context.Background(), 1, question, &meeting.ID)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if answer == "" {
		t.Error("expected non-empty answer")
	}
}

func TestMeetingService_RejectCompletedMeetingRetry(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	meeting, _, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	err := mockRepo.CompleteMeeting(context.Background(), meeting.ID)
	if err != nil {
		t.Fatalf("CompleteMeeting failed: %v", err)
	}

	_, err = svc.RetryProcessing(context.Background(), meeting.ID, 1)
	if err == nil {
		t.Fatal("expected error for retrying completed meeting, got nil")
	}
}

func TestMockSpeechClient_Default(t *testing.T) {
	client := speech.NewMockSpeechClient()
	result, err := client.Recognize(context.Background(), []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("Recognize failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty transcript")
	}
}

func TestMockSpeechClient_Custom(t *testing.T) {
	client := &speech.MockSpeechClient{DefaultTranscript: "Кастомная транскрипция"}
	result, err := client.Recognize(context.Background(), []byte("data"), "audio/mpeg")
	if err != nil {
		t.Fatalf("Recognize failed: %v", err)
	}
	if result != "Кастомная транскрипция" {
		t.Errorf("expected custom transcript, got: %s", result)
	}
}

func TestMockLLMClient_Default(t *testing.T) {
	client := ai.NewMockLLMClient()

	summary, err := client.GetSummary(context.Background(), "test text")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	answer, err := client.Ask(context.Background(), "question", "context")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if answer == "" {
		t.Error("expected non-empty answer")
	}
}

func TestMockLLMClient_Custom(t *testing.T) {
	client := &ai.MockLLMClient{
		DefaultSummary: "Кастомная выжимка",
		DefaultAnswer:  "Кастомный ответ",
	}

	summary, err := client.GetSummary(context.Background(), "test text")
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary != "Кастомная выжимка" {
		t.Errorf("expected custom summary, got: %s", summary)
	}

	answer, err := client.Ask(context.Background(), "question", "context")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if answer != "Кастомный ответ" {
		t.Errorf("expected custom answer, got: %s", answer)
	}
}

func TestMeetingService_ConcurrentAccess(t *testing.T) {
	svc, mockRepo, _, _ := setupTestService(t)

	for i := 1; i <= 10; i++ {
		_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, fmt.Sprintf("meeting_%d.mp3", i))
		if err != nil {
			t.Fatalf("CreateMeetingWithTask failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.ListMeetings(context.Background(), 1)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestMockMeetingRepo_ThreadSafety(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, fmt.Sprintf("meeting_%d.mp3", id))
			if err != nil {
				t.Errorf("CreateMeetingWithTask failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	all := mockRepo.AllMeetings()
	if len(all) != 50 {
		t.Errorf("expected 50 meetings, got %d", len(all))
	}
}

func TestMockMeetingRepo_FailNext(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	mockRepo.SetFailNext(true, fmt.Errorf("simulated error"))
	_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	mockRepo.SetFailNext(false, nil)
	meeting, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test2.mp3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meeting == nil {
		t.Fatal("expected meeting, got nil")
	}
}

func TestMockMeetingRepo_UserIsolation(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	for i := 0; i < 5; i++ {
		_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, fmt.Sprintf("user1_%d.mp3", i))
		if err != nil {
			t.Fatalf("CreateMeetingWithTask failed: %v", err)
		}
		_, _, err = mockRepo.CreateMeetingWithTask(context.Background(), 2, fmt.Sprintf("user2_%d.mp3", i))
		if err != nil {
			t.Fatalf("CreateMeetingWithTask failed: %v", err)
		}
	}

	meetings1, err := mockRepo.ListMeetingsByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListMeetingsByUser failed: %v", err)
	}
	if len(meetings1) != 5 {
		t.Errorf("expected 5 meetings for user 1, got %d", len(meetings1))
	}

	meetings2, err := mockRepo.ListMeetingsByUser(context.Background(), 2)
	if err != nil {
		t.Fatalf("ListMeetingsByUser failed: %v", err)
	}
	if len(meetings2) != 5 {
		t.Errorf("expected 5 meetings for user 2, got %d", len(meetings2))
	}
}

func TestMockMeetingRepo_SearchByKeyword(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	meeting, _, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	transcription := "Обсуждался план проекта на следующий квартал"
	mockRepo.SaveTranscription(context.Background(), meeting.ID, transcription)

	results, err := mockRepo.SearchMeetingsByKeyword(context.Background(), 1, "план")
	if err != nil {
		t.Fatalf("SearchMeetingsByKeyword failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestMockMeetingRepo_UpdateStatus(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	meeting, task, _ := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test.mp3")
	if task.Status != storage.StatusCreated {
		t.Errorf("expected task status created, got %s", task.Status)
	}

	errStr := "test error"
	err := mockRepo.UpdateMeetingStatus(context.Background(), meeting.ID, storage.StatusFailed, &errStr)
	if err != nil {
		t.Fatalf("UpdateMeetingStatus failed: %v", err)
	}

	// Check task status, not meeting status
	finalTask := mockRepo.GetTaskByID(meeting.ID)
	if finalTask == nil {
		t.Fatal("task not found")
	}
	if finalTask.Status != storage.StatusFailed {
		t.Errorf("expected task status failed, got %s", finalTask.Status)
	}
	if finalTask.ErrorMessage == nil || *finalTask.ErrorMessage != errStr {
		t.Errorf("expected error message, got: %v", finalTask.ErrorMessage)
	}
}

func TestMockMeetingRepo_GetMeetingNotFound(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	_, err := mockRepo.GetMeeting(context.Background(), 9999)
	if err == nil {
		t.Fatal("expected error for non-existent meeting")
	}
}

// Note: Integration tests are in internal/storage/integration_test.go
// Run with: INTEGRATION_TEST=true go test ./internal/storage/ -run Integration -v

func TestMockMeetingRepo_TransactionRollback(t *testing.T) {
	mockRepo := NewMockMeetingRepo()

	mockRepo.SetFailNext(false, nil)
	_, _, err := mockRepo.CreateMeetingWithTask(context.Background(), 1, "test1.mp3")
	if err != nil {
		t.Fatalf("first CreateMeetingWithTask failed: %v", err)
	}

	mockRepo.SetFailNext(true, fmt.Errorf("simulated failure"))
	_, _, err = mockRepo.CreateMeetingWithTask(context.Background(), 1, "test2.mp3")
	if err == nil {
		t.Fatal("expected error on second call")
	}

	all := mockRepo.AllMeetings()
	if len(all) != 1 {
		t.Errorf("expected 1 meeting after rollback, got %d", len(all))
	}
}

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
	time.Sleep(100 * time.Millisecond)

	// Shutdown — should wait for in-progress tasks
	svc.Stop()

	// Verify all meetings are in a terminal state (completed or failed)
	all := mockRepo.AllMeetings()
	for id, m := range all {
		switch m.Status {
		case "completed", "failed", "transcribed", "summarized":
			// terminal states — OK
		default:
			t.Errorf("meeting %d in non-terminal state: %s (goroutine leak?)", id, m.Status)
		}
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

func TestNewMeetingService_InvalidOptions(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := ai.NewMockLLMClient()

	// WithWorkers(0) should return error, not panic
	_, err := NewMeetingService(mockRepo, nil, speechClient, llmClient, nil, WithWorkers(0))
	if err == nil {
		t.Fatal("expected error for WithWorkers(0), got nil")
	}
	if !strings.Contains(err.Error(), "workers must be > 0") {
		t.Errorf("expected 'workers must be > 0' error, got: %v", err)
	}

	// WithQueueCapacity(-1) should return error
	_, err = NewMeetingService(mockRepo, nil, speechClient, llmClient, nil, WithQueueCapacity(-1))
	if err == nil {
		t.Fatal("expected error for WithQueueCapacity(-1), got nil")
	}
	if !strings.Contains(err.Error(), "queue capacity must be > 0") {
		t.Errorf("expected 'queue capacity must be > 0' error, got: %v", err)
	}

	// WithTaskTimeout(0) should return error
	_, err = NewMeetingService(mockRepo, nil, speechClient, llmClient, nil, WithTaskTimeout(0))
	if err == nil {
		t.Fatal("expected error for WithTaskTimeout(0), got nil")
	}
	if !strings.Contains(err.Error(), "task timeout must be > 0") {
		t.Errorf("expected 'task timeout must be > 0' error, got: %v", err)
	}
}

func TestNewMeetingService_ValidOptions(t *testing.T) {
	mockRepo := NewMockMeetingRepo()
	speechClient := speech.NewMockSpeechClient()
	llmClient := ai.NewMockLLMClient()

	svc, err := NewMeetingService(
		mockRepo,
		nil,
		speechClient,
		llmClient,
		nil,
		WithWorkers(5),
		WithQueueCapacity(50),
		WithTaskTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error with valid options: %v", err)
	}
	defer svc.Stop()

	if cap(svc.semaphore) != 5 {
		t.Errorf("expected semaphore capacity 5, got %d", cap(svc.semaphore))
	}
	if cap(svc.taskQueue) != 50 {
		t.Errorf("expected task queue capacity 50, got %d", cap(svc.taskQueue))
	}
	if svc.taskTimeout != 30*time.Second {
		t.Errorf("expected task timeout 30s, got %v", svc.taskTimeout)
	}
}
