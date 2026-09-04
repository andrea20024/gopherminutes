package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

// TestCustomErrors_Is checks that custom errors work with errors.Is().
func TestCustomErrors_Is(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		target   error
		expected bool
	}{
		{
			name:     "ErrMeetingNotFound matches",
			err:      WrapMeetingNotFound(fmt.Errorf("underlying error")),
			target:   ErrMeetingNotFound,
			expected: true,
		},
		{
			name:     "ErrAccessDenied matches",
			err:      WrapAccessDenied(fmt.Errorf("underlying error")),
			target:   ErrAccessDenied,
			expected: true,
		},
		{
			name:     "ErrUnsupportedFormat matches",
			err:      WrapUnsupportedFormat(fmt.Errorf("underlying error")),
			target:   ErrUnsupportedFormat,
			expected: true,
		},
		{
			name:     "ErrMeetingNotFound does not match ErrAccessDenied",
			err:      WrapMeetingNotFound(fmt.Errorf("underlying error")),
			target:   ErrAccessDenied,
			expected: false,
		},
		{
			name:     "Plain error does not match custom errors",
			err:      fmt.Errorf("plain error"),
			target:   ErrMeetingNotFound,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := errors.Is(tt.err, tt.target)
			if result != tt.expected {
				t.Errorf("errors.Is() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestCustomErrors_As checks that custom errors work with errors.As().
func TestCustomErrors_As(t *testing.T) {
	t.Run("SpeechClientError transient", func(t *testing.T) {
		err := NewSpeechClientError("service unavailable", true)
		var sce *SpeechClientError
		if !errors.As(err, &sce) {
			t.Error("errors.As() failed to extract SpeechClientError")
		}
		if !sce.Transient {
			t.Error("expected Transient=true, got false")
		}
	})

	t.Run("SpeechClientError permanent", func(t *testing.T) {
		err := NewSpeechClientError("invalid format", false)
		var sce *SpeechClientError
		if !errors.As(err, &sce) {
			t.Error("errors.As() failed to extract SpeechClientError")
		}
		if sce.Transient {
			t.Error("expected Transient=false, got true")
		}
	})

	t.Run("LLMClientError", func(t *testing.T) {
		err := NewLLMClientError("rate limit exceeded", true)
		var lle *LLMClientError
		if !errors.As(err, &lle) {
			t.Error("errors.As() failed to extract LLMClientError")
		}
		if !lle.Transient {
			t.Error("expected Transient=true, got false")
		}
	})

	t.Run("IsSpeechClientError helper", func(t *testing.T) {
		err := NewSpeechClientError("test", false)
		if !IsSpeechClientError(err) {
			t.Error("IsSpeechClientError() returned false for SpeechClientError")
		}
		if IsSpeechClientError(fmt.Errorf("plain error")) {
			t.Error("IsSpeechClientError() returned true for plain error")
		}
	})

	t.Run("IsLLMClientError helper", func(t *testing.T) {
		err := NewLLMClientError("test", false)
		if !IsLLMClientError(err) {
			t.Error("IsLLMClientError() returned false for LLMClientError")
		}
		if IsLLMClientError(fmt.Errorf("plain error")) {
			t.Error("IsLLMClientError() returned true for plain error")
		}
	})
}

// TestErrorMessages checks that error messages are user-friendly.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectedSub string
	}{
		{
			name:        "ErrMeetingNotFound message",
			err:         WrapMeetingNotFound(fmt.Errorf("sql: no rows")),
			expectedSub: "meeting not found",
		},
		{
			name:        "ErrAccessDenied message",
			err:         WrapAccessDenied(fmt.Errorf("user mismatch")),
			expectedSub: "access denied",
		},
		{
			name:        "ErrUnsupportedFormat message",
			err:         WrapUnsupportedFormat(fmt.Errorf("bad format")),
			expectedSub: "unsupported file format",
		},
		{
			name:        "ErrSpeechClientUnavailable message",
			err:         ErrSpeechClientUnavailable,
			expectedSub: "speech recognition service unavailable",
		},
		{
			name:        "ErrLLMClientUnavailable message",
			err:         ErrLLMClientUnavailable,
			expectedSub: "LLM service unavailable",
		},
		{
			name:        "ErrRetryOnNonFailedMeeting message",
			err:         fmt.Errorf("%w: current status is completed", ErrRetryOnNonFailedMeeting),
			expectedSub: "retry only works for failed meetings",
		},
		{
			name:        "ErrGridFSFileNotFound message",
			err:         fmt.Errorf("%w: file not found", ErrGridFSFileNotFound),
			expectedSub: "file not found in GridFS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			if errMsg == "" {
				t.Error("error message is empty")
			}
			if !contains(errMsg, tt.expectedSub) {
				t.Errorf("error message %q does not contain %q", errMsg, tt.expectedSub)
			}
		})
	}
}

// TestValidateFileFormat tests the ValidateFileFormat function.
func TestValidateFileFormat(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errSub  string
	}{
		{
			name:    "mp3 file",
			path:    "/path/to/meeting.mp3",
			wantErr: false,
		},
		{
			name:    "wav file",
			path:    "/path/to/meeting.wav",
			wantErr: false,
		},
		{
			name:    "ogg file",
			path:    "/path/to/meeting.ogg",
			wantErr: false,
		},
		{
			name:    "flac file",
			path:    "/path/to/meeting.flac",
			wantErr: false,
		},
		{
			name:    "unsupported txt file",
			path:    "/path/to/meeting.txt",
			wantErr: true,
			errSub:  "unsupported file format",
		},
		{
			name:    "unsupported pdf file",
			path:    "/path/to/meeting.pdf",
			wantErr: true,
			errSub:  "unsupported file format",
		},
		{
			name:    "no extension",
			path:    "/path/to/meeting",
			wantErr: true,
			errSub:  "unsupported file format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileFormat(tt.path)
			if tt.wantErr && err == nil {
				t.Error("ValidateFileFormat() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateFileFormat() unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !contains(err.Error(), tt.errSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errSub)
			}
		})
	}
}

// TestGetFileExtension tests filepath.Ext behavior.
func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "mp3 extension",
			path: "/path/to/meeting.mp3",
			want: ".mp3",
		},
		{
			name: "wav extension",
			path: "/path/to/meeting.wav",
			want: ".wav",
		},
		{
			name: "no extension",
			path: "/path/to/meeting",
			want: "",
		},
		{
			name: "multiple dots",
			path: "/path/to/meeting.2024.01.01.mp3",
			want: ".mp3",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name: "tar.gz",
			path: "/path/to/archive.tar.gz",
			want: ".gz",
		},
		{
			name: "hidden file",
			path: "/path/to/.hidden",
			want: ".hidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filepath.Ext(tt.path)
			if got != tt.want {
				t.Errorf("filepath.Ext(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestIsSupportedFormat tests the IsSupportedFormat function.
func TestIsSupportedFormat(t *testing.T) {
	tests := []struct {
		ext       string
		supported bool
	}{
		{".mp3", true},
		{".wav", true},
		{".ogg", true},
		{".flac", true},
		{".txt", false},
		{".pdf", false},
		{".jpg", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := IsSupportedFormat(tt.ext)
			if got != tt.supported {
				t.Errorf("IsSupportedFormat(%q) = %v, want %v", tt.ext, got, tt.supported)
			}
		})
	}
}

// TestWrapFunctions tests that wrap functions preserve the underlying error.
func TestWrapFunctions(t *testing.T) {
	underlying := fmt.Errorf("underlying error")

	t.Run("WrapMeetingNotFound preserves underlying", func(t *testing.T) {
		wrapped := WrapMeetingNotFound(underlying)
		if !errors.Is(wrapped, ErrMeetingNotFound) {
			t.Error("wrapped error does not match ErrMeetingNotFound")
		}
		if !errors.Is(wrapped, underlying) {
			t.Error("wrapped error does not preserve underlying error")
		}
	})

	t.Run("WrapAccessDenied preserves underlying", func(t *testing.T) {
		wrapped := WrapAccessDenied(underlying)
		if !errors.Is(wrapped, ErrAccessDenied) {
			t.Error("wrapped error does not match ErrAccessDenied")
		}
		if !errors.Is(wrapped, underlying) {
			t.Error("wrapped error does not preserve underlying error")
		}
	})

	t.Run("WrapUnsupportedFormat preserves underlying", func(t *testing.T) {
		wrapped := WrapUnsupportedFormat(underlying)
		if !errors.Is(wrapped, ErrUnsupportedFormat) {
			t.Error("wrapped error does not match ErrUnsupportedFormat")
		}
		if !errors.Is(wrapped, underlying) {
			t.Error("wrapped error does not preserve underlying error")
		}
	})
}

// TestIdiomaticErrorHandling tests idiomatic Go error handling patterns.
func TestIdiomaticErrorHandling(t *testing.T) {
	t.Run("errors.Is with wrapped error", func(t *testing.T) {
		// Simulate the storage layer wrapping sql.ErrNoRows
		storageErr := fmt.Errorf("get meeting: %w", fmt.Errorf("sql: no rows"))

		// Service layer wraps it further
		serviceErr := fmt.Errorf("get meeting: %w", storageErr)

		// Custom error wrapping
		customErr := WrapMeetingNotFound(serviceErr)

		// Should be able to check custom error
		if !errors.Is(customErr, ErrMeetingNotFound) {
			t.Error("should detect ErrMeetingNotFound through multiple wrap levels")
		}
	})

	t.Run("errors.As with custom error type", func(t *testing.T) {
		err := NewSpeechClientError("service unavailable", true)

		var sce *SpeechClientError
		if !errors.As(err, &sce) {
			t.Error("should extract SpeechClientError")
		}

		if !sce.Transient {
			t.Error("should preserve Transient field")
		}
	})

	t.Run("double wrap does not duplicate custom error", func(t *testing.T) {
		inner := WrapMeetingNotFound(fmt.Errorf("underlying"))
		outer := WrapMeetingNotFound(inner)

		// Should still match
		if !errors.Is(outer, ErrMeetingNotFound) {
			t.Error("errors.Is should work through double wrap")
		}
	})
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
