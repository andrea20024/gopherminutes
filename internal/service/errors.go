// Package service provides business logic for meeting processing.
package service

import (
	"errors"
	"fmt"
)

// Application-level errors for structured error handling.
// These errors can be checked using errors.Is() and errors.As().
var (
	// ErrMeetingNotFound is returned when a meeting with the given ID does not exist.
	ErrMeetingNotFound = errors.New("meeting not found")

	// ErrAccessDenied is returned when a user tries to access another user's meeting.
	ErrAccessDenied = errors.New("access denied: meeting does not belong to this user")

	// ErrUnsupportedFormat is returned when the file format is not supported.
	ErrUnsupportedFormat = errors.New("unsupported file format")

	// ErrSpeechClientUnavailable is returned when the speech recognition service is unavailable.
	ErrSpeechClientUnavailable = errors.New("speech recognition service unavailable")

	// ErrLLMClientUnavailable is returned when the LLM service is unavailable.
	ErrLLMClientUnavailable = errors.New("LLM service unavailable")

	// ErrDatabaseUnavailable is returned when the database connection is lost.
	ErrDatabaseUnavailable = errors.New("database unavailable")

	// ErrServiceShuttingDown is returned when the service is in the process of shutting down.
	ErrServiceShuttingDown = errors.New("service shutting down")

	// ErrRetryOnNonFailedMeeting is returned when retry is attempted on a non-failed meeting.
	ErrRetryOnNonFailedMeeting = errors.New("retry only works for failed meetings")

	// ErrGridFSFileNotFound is returned when a file is not found in GridFS.
	ErrGridFSFileNotFound = errors.New("file not found in GridFS")

	// ErrGridFSNotConfigured is returned when GridFS is not configured.
	ErrGridFSNotConfigured = errors.New("GridFS is not configured")

	// ErrFileNotFound is returned when the specified file does not exist on disk.
	ErrFileNotFound = errors.New("file not found on disk")

	// ErrInvalidMeetingID is returned when the meeting ID is invalid.
	ErrInvalidMeetingID = errors.New("invalid meeting ID")

	// ErrContextTimeout is returned when a context deadline is exceeded.
	ErrContextTimeout = errors.New("context deadline exceeded")

	// ErrUnknownCommand is returned when an unknown command is received.
	ErrUnknownCommand = errors.New("unknown command")

	// ErrMissingArguments is returned when required arguments are missing.
	ErrMissingArguments = errors.New("missing required arguments")
)

// SpeechClientError represents an error from the speech recognition client.
type SpeechClientError struct {
	Message   string
	Transient bool // true if the error might resolve on retry
}

func (e *SpeechClientError) Error() string {
	return e.Message
}

func (e *SpeechClientError) Unwrap() error {
	return nil
}

// LLMClientError represents an error from the LLM client.
type LLMClientError struct {
	Message   string
	Transient bool // true if the error might resolve on retry
}

func (e *LLMClientError) Error() string {
	return e.Message
}

func (e *LLMClientError) Unwrap() error {
	return nil
}

// IsSpeechClientError checks if an error is a SpeechClientError.
func IsSpeechClientError(err error) bool {
	var sce *SpeechClientError
	return errors.As(err, &sce)
}

// IsLLMClientError checks if an error is an LLMClientError.
func IsLLMClientError(err error) bool {
	var lle *LLMClientError
	return errors.As(err, &lle)
}

// NewSpeechClientError creates a new SpeechClientError.
func NewSpeechClientError(message string, transient bool) *SpeechClientError {
	return &SpeechClientError{
		Message:   message,
		Transient: transient,
	}
}

// NewLLMClientError creates a new LLMClientError.
func NewLLMClientError(message string, transient bool) *LLMClientError {
	return &LLMClientError{
		Message:   message,
		Transient: transient,
	}
}

// WrapMeetingNotFound wraps an error with ErrMeetingNotFound.
func WrapMeetingNotFound(err error) error {
	if errors.Is(err, ErrMeetingNotFound) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrMeetingNotFound, err)
}

// WrapAccessDenied wraps an error with ErrAccessDenied.
func WrapAccessDenied(err error) error {
	if errors.Is(err, ErrAccessDenied) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrAccessDenied, err)
}

// WrapUnsupportedFormat wraps an error with ErrUnsupportedFormat.
func WrapUnsupportedFormat(err error) error {
	if errors.Is(err, ErrUnsupportedFormat) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnsupportedFormat, err)
}
