//go:build integration

package storage

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	dbURL := os.Getenv("INTEGRATION_TEST_DB_URL")
	if dbURL == "" {
		dbURL = "postgres://loader:1234@localhost:5434/truecode_db?sslmode=disable"
	}
	var err error
	testDB, err = sql.Open("pgx", dbURL)
	if err != nil {
		panic(err)
	}
	defer testDB.Close()

	code := m.Run()
	os.Exit(code)
}

func setupIntegrationTest(t *testing.T) (*UserRepo, *MeetingRepo, func()) {
	t.Helper()
	ctx := context.Background()

	repo := NewRepository(testDB)
	userRepo := NewUserRepo(repo)
	meetingRepo := NewMeetingRepo(repo)

	cleanup := func() {
		testDB.ExecContext(ctx, "DELETE FROM meeting_tasks")
		testDB.ExecContext(ctx, "DELETE FROM meetings")
		testDB.ExecContext(ctx, "DELETE FROM users")
	}

	return userRepo, meetingRepo, cleanup
}

func TestIntegration_CreateMeetingWithTask(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	user, err := userRepo.CreateUser(context.Background(), 100, "testuser")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	meeting, task, err := meetingRepo.CreateMeetingWithTask(context.Background(), user.ID, "test.mp3")
	if err != nil {
		t.Fatalf("CreateMeetingWithTask failed: %v", err)
	}

	if meeting.ID == 0 {
		t.Error("expected meeting ID > 0")
	}
	if task.ID == 0 {
		t.Error("expected task ID > 0")
	}
	if meeting.Status != StatusCreated {
		t.Errorf("expected status created, got %s", meeting.Status)
	}
}

func TestIntegration_SaveTranscription(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	user, _ := userRepo.CreateUser(context.Background(), 101, "transuser")
	meeting, _, _ := meetingRepo.CreateMeetingWithTask(context.Background(), user.ID, "test.mp3")

	err := meetingRepo.UpdateMeetingStatus(context.Background(), meeting.ID, StatusProcessing, nil)
	if err != nil {
		t.Fatalf("UpdateMeetingStatus failed: %v", err)
	}

	err = meetingRepo.SaveTranscription(context.Background(), meeting.ID, "Тестовая транскрипция")
	if err != nil {
		t.Fatalf("SaveTranscription failed: %v", err)
	}

	m, err := meetingRepo.GetMeeting(context.Background(), meeting.ID)
	if err != nil {
		t.Fatalf("GetMeeting failed: %v", err)
	}

	if m.Transcription == nil || *m.Transcription != "Тестовая транскрипция" {
		t.Error("expected transcription to be set")
	}
	if m.Status != StatusTranscribed {
		t.Errorf("expected status transcribed, got %s", m.Status)
	}
}

func TestIntegration_SaveSummary(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	user, _ := userRepo.CreateUser(context.Background(), 102, "summaryuser")
	meeting, _, _ := meetingRepo.CreateMeetingWithTask(context.Background(), user.ID, "test.mp3")
	meetingRepo.SaveTranscription(context.Background(), meeting.ID, "Транскрипция")

	err := meetingRepo.SaveSummary(context.Background(), meeting.ID, "Краткая выжимка")
	if err != nil {
		t.Fatalf("SaveSummary failed: %v", err)
	}

	m, err := meetingRepo.GetMeeting(context.Background(), meeting.ID)
	if err != nil {
		t.Fatalf("GetMeeting failed: %v", err)
	}

	if m.Summary == nil || *m.Summary != "Краткая выжимка" {
		t.Error("expected summary to be set")
	}
	if m.Status != StatusSummarized {
		t.Errorf("expected status summarized, got %s", m.Status)
	}
}

func TestIntegration_SearchByKeyword(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	user, _ := userRepo.CreateUser(context.Background(), 103, "searchuser")

	m1, _, _ := meetingRepo.CreateMeetingWithTask(context.Background(), user.ID, "file1.mp3")
	meetingRepo.SaveTranscription(context.Background(), m1.ID, "обсуждение проекта и планы на завтра")

	m2, _, _ := meetingRepo.CreateMeetingWithTask(context.Background(), user.ID, "file2.mp3")
	meetingRepo.SaveTranscription(context.Background(), m2.ID, "обсуждение бюджета на квартал")

	results, err := meetingRepo.SearchMeetingsByKeyword(context.Background(), user.ID, "бюджет")
	if err != nil {
		t.Fatalf("SearchMeetingsByKeyword failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != m2.ID {
		t.Errorf("expected meeting ID %d, got %d", m2.ID, results[0].ID)
	}
}

func TestIntegration_UserDataIsolation(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	user1, _ := userRepo.CreateUser(context.Background(), 200, "user1")
	user2, _ := userRepo.CreateUser(context.Background(), 201, "user2")

	_, _, _ = meetingRepo.CreateMeetingWithTask(context.Background(), user1.ID, "user1.mp3")
	_, _, _ = meetingRepo.CreateMeetingWithTask(context.Background(), user2.ID, "user2.mp3")

	meetings1, err := meetingRepo.ListMeetingsByUser(context.Background(), user1.ID)
	if err != nil {
		t.Fatalf("ListMeetingsByUser failed: %v", err)
	}
	if len(meetings1) != 1 {
		t.Errorf("expected 1 meeting for user 1, got %d", len(meetings1))
	}

	meetings2, err := meetingRepo.ListMeetingsByUser(context.Background(), user2.ID)
	if err != nil {
		t.Fatalf("ListMeetingsByUser failed: %v", err)
	}
	if len(meetings2) != 1 {
		t.Errorf("expected 1 meeting for user 2, got %d", len(meetings2))
	}
}

func TestIntegration_FullLifecycle(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()

	user, _ := userRepo.CreateUser(context.Background(), 300, "lifecycle")
	m, _, _ := meetingRepo.CreateMeetingWithTask(ctx, user.ID, "lifecycle.mp3")
	if m.Status != StatusCreated {
		t.Errorf("expected created, got %s", m.Status)
	}

	meetingRepo.UpdateMeetingStatus(ctx, m.ID, StatusProcessing, nil)
	meetingRepo.SaveTranscription(ctx, m.ID, "Полная транскрипция встречи тест")
	meetingRepo.SaveSummary(ctx, m.ID, "Полная выжимка по встрече")
	meetingRepo.CompleteMeeting(ctx, m.ID)

	updated, err := meetingRepo.GetMeeting(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMeeting failed: %v", err)
	}

	if updated.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", updated.Status)
	}
	if updated.Transcription == nil || *updated.Transcription != "Полная транскрипция встречи тест" {
		t.Error("expected transcription to be set")
	}
	if updated.Summary == nil || *updated.Summary != "Полная выжимка по встрече" {
		t.Error("expected summary to be set")
	}
}

func TestIntegration_FailedStatus(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	user, _ := userRepo.CreateUser(context.Background(), 400, "faileduser")
	m, _, _ := meetingRepo.CreateMeetingWithTask(ctx, user.ID, "failed.mp3")

	errMsg := "speech client timeout"
	meetingRepo.UpdateMeetingStatus(ctx, m.ID, StatusFailed, &errMsg)

	updated, err := meetingRepo.GetMeeting(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetMeeting failed: %v", err)
	}

	if updated.Status != StatusFailed {
		t.Errorf("expected failed, got %s", updated.Status)
	}
	if updated.ErrorMessage == nil || *updated.ErrorMessage != errMsg {
		t.Errorf("expected error message, got: %v", updated.ErrorMessage)
	}
}

func TestIntegration_ListMeetingsWithStatuses(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("skip integration test - set INTEGRATION_TEST=true")
	}

	userRepo, meetingRepo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	user, _ := userRepo.CreateUser(context.Background(), 500, "listuser")

	meetingRepo.CreateMeetingWithTask(ctx, user.ID, "file1.mp3")
	meetingRepo.CreateMeetingWithTask(ctx, user.ID, "file2.mp3")
	meetingRepo.CreateMeetingWithTask(ctx, user.ID, "file3.mp3")

	meetings, err := meetingRepo.ListMeetingsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListMeetingsByUser failed: %v", err)
	}

	if len(meetings) != 3 {
		t.Errorf("expected 3 meetings, got %d", len(meetings))
	}

	if meetings[0].ID < meetings[1].ID {
		t.Error("meetings should be sorted by created_at DESC")
	}
}
