// Package ai provides LLM client implementations for text summarization and Q&A.
package ai

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// LLMClient defines the interface for LLM-based text processing.
// Implementations can be real (GigaChat) or mock for testing.
type LLMClient interface {
	// GetSummary generates a brief summary of the given text.
	GetSummary(ctx context.Context, text string) (string, error)

	// Ask answers a question based on the given context (transcription text).
	Ask(ctx context.Context, question string, contextText string) (string, error)
}

// MockLLMClient returns hardcoded summary and answers for testing.
type MockLLMClient struct {
	DefaultSummary string
	DefaultAnswer  string
	SummaryError   error // if set, GetSummary returns this error
	AskError       error // if set, Ask returns this error
}

// NewMockLLMClient creates a new MockLLMClient with default responses.
func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		DefaultSummary: "Краткая выжимка: на встрече обсуждались вопросы плана проекта, были распределены задачи между участниками.",
		DefaultAnswer:  "На основе материалов встречи: обсуждались вопросы плана проекта и распределения задач между участниками.",
	}
}

// GetSummary returns a hardcoded summary or a configured error.
func (m *MockLLMClient) GetSummary(ctx context.Context, text string) (string, error) {
	if m.SummaryError != nil {
		return "", m.SummaryError
	}
	if m.DefaultSummary == "" {
		return "Краткая выжимка по встрече.", nil
	}
	return m.DefaultSummary, nil
}

// Ask returns a hardcoded answer or a configured error.
func (m *MockLLMClient) Ask(ctx context.Context, question string, contextText string) (string, error) {
	if m.AskError != nil {
		return "", m.AskError
	}
	if m.DefaultAnswer == "" {
		return "Ответ на ваш вопрос на основе материалов встречи.", nil
	}
	return m.DefaultAnswer, nil
}

// Compile-time interface check.
var _ LLMClient = (*MockLLMClient)(nil)

// SlowMockLLMClient returns responses after a configurable delay.
// Useful for testing context cancellation and timeout behavior.
type SlowMockLLMClient struct {
	DefaultSummary string
	DefaultAnswer  string
	Delay          time.Duration
}

// NewSlowMockLLMClient creates a slow mock client with the given delay.
func NewSlowMockLLMClient(delay time.Duration) *SlowMockLLMClient {
	return &SlowMockLLMClient{
		DefaultSummary: "Краткая выжимка: на встрече обсуждались вопросы плана проекта, были распределены задачи между участниками.",
		DefaultAnswer:  "На основе материалов встречи: обсуждались вопросы плана проекта и распределения задач между участниками.",
		Delay:          delay,
	}
}

// GetSummary respects context cancellation and delays before returning.
func (m *SlowMockLLMClient) GetSummary(ctx context.Context, text string) (string, error) {
	select {
	case <-time.After(m.Delay):
		if m.DefaultSummary == "" {
			return "Краткая выжимка по встрече.", nil
		}
		return m.DefaultSummary, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Ask respects context cancellation and delays before returning.
func (m *SlowMockLLMClient) Ask(ctx context.Context, question string, contextText string) (string, error) {
	select {
	case <-time.After(m.Delay):
		if m.DefaultAnswer == "" {
			return "Ответ на ваш вопрос на основе материалов встречи.", nil
		}
		return m.DefaultAnswer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Compile-time interface check.
var _ LLMClient = (*SlowMockLLMClient)(nil)

// GigaChatClient implements LLMClient for GigaChat API.
type GigaChatClient struct {
	apiKey string
	client *http.Client
}

// NewGigaChatClient creates a new GigaChatClient.
func NewGigaChatClient(apiKey string) *GigaChatClient {
	return &GigaChatClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetSummary generates a summary via GigaChat API.
func (c *GigaChatClient) GetSummary(ctx context.Context, text string) (string, error) {
	return "", fmt.Errorf("GigaChat integration not yet implemented")
}

// Ask answers a question via GigaChat API.
func (c *GigaChatClient) Ask(ctx context.Context, question string, contextText string) (string, error) {
	return "", fmt.Errorf("GigaChat integration not yet implemented")
}
