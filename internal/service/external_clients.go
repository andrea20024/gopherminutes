// Package service provides business logic for meeting processing.
package service

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

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

// GetMIMEType returns the MIME type for a file extension.
func GetMIMEType(ext string) string {
	switch ext {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}

// SupportedFormats defines the list of supported file extensions.
var SupportedFormats = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".ogg":  true,
	".flac": true,
}

// IsSupportedFormat checks if the file extension is in the supported formats list.
func IsSupportedFormat(ext string) bool {
	return SupportedFormats[ext]
}

// ValidateFileFormat checks if the file format is supported and returns an error if not.
func ValidateFileFormat(filePath string) error {
	ext := filepath.Ext(filePath)
	if !IsSupportedFormat(ext) {
		return fmt.Errorf("%w: '%s' (supported: mp3, wav, ogg, flac)", ErrUnsupportedFormat, ext)
	}
	return nil
}
