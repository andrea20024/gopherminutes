// Package speech provides speech recognition client implementations.
package speech

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// SpeechClient defines the interface for speech recognition services.
// Implementations can be real (SaluteSpeech, Yandex) or mock for testing.
type SpeechClient interface {
	// Recognize transcribes audio data and returns the text transcript.
	Recognize(ctx context.Context, data []byte, mime string) (string, error)
}

// MockSpeechClient returns a hardcoded transcript for testing.
type MockSpeechClient struct {
	DefaultTranscript string
	Error             error // if set, Recognize returns this error
}

// NewMockSpeechClient creates a new MockSpeechClient with a default transcript.
func NewMockSpeechClient() *MockSpeechClient {
	return &MockSpeechClient{
		DefaultTranscript: "Это тестовая транскрипция встречи. Обсуждались вопросы плана проекта и распределения задач.",
	}
}

// Recognize returns the hardcoded transcript or a configured error.
func (m *MockSpeechClient) Recognize(ctx context.Context, data []byte, mime string) (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	if m.DefaultTranscript == "" {
		return "Это тестовая транскрипция встречи.", nil
	}
	return m.DefaultTranscript, nil
}

// Compile-time interface check.
var _ SpeechClient = (*MockSpeechClient)(nil)

// SlowMockSpeechClient returns a transcript after a configurable delay.
// Useful for testing context cancellation and timeout behavior.
type SlowMockSpeechClient struct {
	DefaultTranscript string
	Delay             time.Duration
}

// NewSlowMockSpeechClient creates a slow mock client with the given delay.
func NewSlowMockSpeechClient(delay time.Duration) *SlowMockSpeechClient {
	return &SlowMockSpeechClient{
		DefaultTranscript: "Это тестовая транскрипция встречи. Обсуждались вопросы плана проекта и распределения задач.",
		Delay:             delay,
	}
}

// Recognize respects context cancellation and delays before returning.
func (m *SlowMockSpeechClient) Recognize(ctx context.Context, data []byte, mime string) (string, error) {
	select {
	case <-time.After(m.Delay):
		if m.DefaultTranscript == "" {
			return "Это тестовая транскрипция встречи.", nil
		}
		return m.DefaultTranscript, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Compile-time interface check.
var _ SpeechClient = (*SlowMockSpeechClient)(nil)

// CountingSpeechClient tracks max concurrent calls for semaphore verification.
type CountingSpeechClient struct {
	current int64
	max     int64
	delay   time.Duration
}

// NewCountingSpeechClient creates a client that tracks concurrency.
func NewCountingSpeechClient(delay time.Duration) *CountingSpeechClient {
	return &CountingSpeechClient{delay: delay}
}

// Current returns the number of concurrent calls.
func (c *CountingSpeechClient) Current() int64 {
	return atomic.LoadInt64(&c.current)
}

// Max returns the max concurrent calls observed.
func (c *CountingSpeechClient) Max() int64 {
	return atomic.LoadInt64(&c.max)
}

// Recognize increments counter, sleeps, then decrements.
func (c *CountingSpeechClient) Recognize(ctx context.Context, data []byte, mime string) (string, error) {
	current := atomic.AddInt64(&c.current, 1)
	max := atomic.LoadInt64(&c.max)
	for current > max && !atomic.CompareAndSwapInt64(&c.max, max, current) {
		max = atomic.LoadInt64(&c.max)
	}
	select {
	case <-time.After(c.delay):
		atomic.AddInt64(&c.current, -1)
		return "transcript", nil
	case <-ctx.Done():
		atomic.AddInt64(&c.current, -1)
		return "", ctx.Err()
	}
}

// Compile-time interface check.
var _ SpeechClient = (*CountingSpeechClient)(nil)

// SaluteSpeechClient implements SpeechClient for SaluteSpeech API.
type SaluteSpeechClient struct {
	apiKey string
	client *http.Client
}

// NewSaluteSpeechClient creates a new SaluteSpeechClient.
func NewSaluteSpeechClient(apiKey string) *SaluteSpeechClient {
	return &SaluteSpeechClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Recognize transcribes audio via SaluteSpeech API.
func (c *SaluteSpeechClient) Recognize(ctx context.Context, data []byte, mime string) (string, error) {
	return "", fmt.Errorf("SaluteSpeech integration not yet implemented")
}
