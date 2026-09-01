// Package interfaces defines contracts for external service clients.
package interfaces

import "context"

// SpeechClient defines the interface for speech recognition services.
// Implementations can be real (SaluteSpeech, Yandex) or mock for testing.
type SpeechClient interface {
	// Recognize transcribes audio data and returns the text transcript.
	Recognize(ctx context.Context, data []byte, mime string) (string, error)
}

// LLMClient defines the interface for LLM-based text processing.
// Implementations can be real (GigaChat) or mock for testing.
type LLMClient interface {
	// GetSummary generates a brief summary of the given text.
	GetSummary(ctx context.Context, text string) (string, error)

	// Ask answers a question based on the given context (transcription text).
	Ask(ctx context.Context, question string, contextText string) (string, error)
}
