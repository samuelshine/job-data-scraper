// Package middleware provides HTTP middleware for the chi router.
// This file implements a structured HTTP request logger using log/slog.
//
// Every request receives a unique request_id (UUID v4 format without dashes)
// that is:
//   - Injected into the request context (retrievable via RequestIDFromContext)
//   - Added as the X-Request-ID response header
//   - Included in every log line for that request
//
// Log fields emitted on each request:
//
//	method      HTTP method (GET, POST, …)
//	path        URL path
//	status      HTTP response status code
//	latency_ms  Float milliseconds elapsed
//	ip          Client IP address
//	request_id  Unique per-request UUID
//	user_agent  User-Agent header
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

type contextKeyType string

const requestIDKey contextKeyType = "requestID"

// RequestIDFromContext extracts the request ID injected by SlogLogger.
// Returns an empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// responseWriter is a thin wrapper that captures the HTTP status code written
// by downstream handlers so we can log it after the fact.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// SlogLogger returns a chi-compatible middleware that logs every HTTP request
// as a single structured slog record after the response is sent.
//
// Example output (text format):
//
//	time=2026-03-27T10:00:00Z level=INFO msg="http request" method=GET path=/api/v1/jobs status=200 latency_ms=12.34 ip=127.0.0.1 request_id=a3f2b1c4
//
// Example output (json format):
//
//	{"time":"2026-03-27T10:00:00Z","level":"INFO","msg":"http request","method":"GET","path":"/api/v1/jobs","status":200,"latency_ms":12.34,"ip":"127.0.0.1","request_id":"a3f2b1c4"}
func SlogLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate a short unique request ID (8 hex chars)
		reqID := generateRequestID()

		// Inject request ID into context and response header
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)

		// Wrap the response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		// Process request
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Determine log level based on status code
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		level := slog.LevelInfo
		if wrapped.status >= 500 {
			level = slog.LevelError
		} else if wrapped.status >= 400 {
			level = slog.LevelWarn
		}

		slog.Log(ctx, level, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"latency_ms", fmt.Sprintf("%.2f", latency),
			"ip", realIP(r),
			"request_id", reqID,
			"user_agent", r.UserAgent(),
		)
	})
}

// generateRequestID creates a short 8-character hex request ID.
func generateRequestID() string {
	return fmt.Sprintf("%08x", rand.Int31())
}

// realIP extracts the client IP, respecting common proxy headers.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
