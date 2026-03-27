// Package logger provides a shared structured logger for the application.
// It wraps Go 1.21+ log/slog with environment-aware formatting:
//   - In development (LOG_FORMAT=text or default), output is human-readable text.
//   - In production (LOG_FORMAT=json), output is structured JSON suitable for
//     log aggregators like Datadog, Loki, or Google Cloud Logging.
//
// Usage:
//
//	logger.Info("scrape complete", "source", name, "count", len(jobs))
//	logger.Error("db error", "err", err)
package logger

import (
	"log/slog"
	"os"
	"strings"
)

var (
	// L is the package-level structured logger. Use this throughout the app
	// instead of the standard log.Printf.
	L *slog.Logger
)

func init() {
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     resolveLevel(),
		AddSource: false,
	}

	if format == "json" {
		// Structured JSON for production / log aggregators
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		// Human-readable text for development
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	L = slog.New(handler)
	slog.SetDefault(L)
}

// resolveLevel reads LOG_LEVEL from the environment.
// Supported values: debug, info (default), warn, error.
func resolveLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	L.Info(msg, args...)
}

// Debug logs a debug-level message.
func Debug(msg string, args ...any) {
	L.Debug(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	L.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	L.Error(msg, args...)
}
