// Package service provides business logic for meeting processing.
package service

import (
	"fmt"
	"path/filepath"
)

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
