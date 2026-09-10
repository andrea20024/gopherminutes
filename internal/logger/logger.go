// Package logger provides structured logging using Uber Zap.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	sugar *zap.SugaredLogger
	log   *zap.Logger
)

// InitLogger initializes the global logger with production console encoding.
func InitLogger(level zapcore.Level) error {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err := cfg.Build()
	if err != nil {
		return err
	}

	log = l
	sugar = l.Sugar()
	return nil
}

// Sugar returns the global SugaredLogger instance.
func Sugar() *zap.SugaredLogger {
	return sugar
}

// Log returns the global *zap.Logger instance.
func Log() *zap.Logger {
	return log
}

// Sync flushes any buffered log entries.
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}
