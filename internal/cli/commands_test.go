package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/andrea20024/goferminutes2/internal/service"
)

func TestFormatError_AllCases(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantSub string
		wantNil bool
	}{
		{
			name:    "ErrMeetingNotFound returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrMeetingNotFound),
			wantSub: "meeting not found",
		},
		{
			name:    "ErrAccessDenied returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrAccessDenied),
			wantSub: "access denied",
		},
		{
			name:    "ErrUnsupportedFormat returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrUnsupportedFormat),
			wantSub: "unsupported file format",
		},
		{
			name:    "ErrFileNotFound returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrFileNotFound),
			wantSub: "file not found",
		},
		{
			name:    "ErrRetryOnNonFailedMeeting returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrRetryOnNonFailedMeeting),
			wantSub: "retry is only available for failed meetings",
		},
		{
			name:    "ErrGridFSFileNotFound returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrGridFSFileNotFound),
			wantSub: "audio file not found in storage",
		},
		{
			name:    "ErrGridFSNotConfigured returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrGridFSNotConfigured),
			wantSub: "GridFS is not configured",
		},
		{
			name:    "ErrServiceShuttingDown returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrServiceShuttingDown),
			wantSub: "service is shutting down",
		},
		{
			name:    "ErrContextTimeout returns user-friendly message",
			err:     fmt.Errorf("test: %w", service.ErrContextTimeout),
			wantSub: "operation timed out",
		},
		{
			name:    "Plain error returns as-is",
			err:     fmt.Errorf("some unknown error"),
			wantSub: "some unknown error",
		},
		{
			name:    "Nil error returns nil",
			err:     nil,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatError(tt.err)

			if tt.wantNil {
				if result != nil {
					t.Errorf("formatError(nil) = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("formatError returned nil, want error")
			}

			resultStr := result.Error()
			if resultStr == "" {
				t.Error("formatError returned empty error message")
			}

			// Check that the result contains the expected substring
			// For ErrRetryOnNonFailedMeeting, the message includes the current status
			if tt.name == "ErrRetryOnNonFailedMeeting returns user-friendly message" {
				if !strings.Contains(resultStr, "retry is only available for failed meetings") {
					t.Errorf("error %q does not contain 'retry is only available for failed meetings'", resultStr)
				}
			} else if !strings.Contains(resultStr, tt.wantSub) {
				t.Errorf("error %q does not contain %q", resultStr, tt.wantSub)
			}
		})
	}
}

func TestFormatError_WrappedErrors(t *testing.T) {
	// Test that wrapped errors still match correctly
	wrappedMeetingNotFound := fmt.Errorf("get meeting: %w", service.WrapMeetingNotFound(fmt.Errorf("sql: no rows")))
	result := formatError(wrappedMeetingNotFound)
	if result == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(result.Error(), "meeting not found") {
		t.Errorf("wrapped ErrMeetingNotFound not recognized: %v", result)
	}

	wrappedAccessDenied := fmt.Errorf("service: %w", service.WrapAccessDenied(fmt.Errorf("user mismatch")))
	result = formatError(wrappedAccessDenied)
	if result == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(result.Error(), "access denied") {
		t.Errorf("wrapped ErrAccessDenied not recognized: %v", result)
	}
}

func TestFormatError_SpecificErrorTypes(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantSub string
	}{
		{"ErrSpeechClientUnavailable", service.ErrSpeechClientUnavailable, "speech recognition service unavailable"},
		{"ErrLLMClientUnavailable", service.ErrLLMClientUnavailable, "LLM service unavailable"},
		{"ErrDatabaseUnavailable", service.ErrDatabaseUnavailable, "database connection failed"},
		{"ErrInvalidMeetingID", service.ErrInvalidMeetingID, "invalid meeting ID"},
		{"ErrUnknownCommand", service.ErrUnknownCommand, "unknown command"},
		{"ErrMissingArguments", service.ErrMissingArguments, "missing required arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatError(tt.err)
			if result == nil {
				t.Fatal("expected non-nil error")
			}
			if !strings.Contains(result.Error(), tt.wantSub) {
				t.Errorf("formatError(%v) = %q, want to contain %q", tt.err, result.Error(), tt.wantSub)
			}
		})
	}
}
