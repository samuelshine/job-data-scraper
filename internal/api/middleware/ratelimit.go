// Package middleware provides HTTP middleware for the chi router.
// This file implements an IP-based rate limiter using the Token Bucket algorithm,
// built entirely with the Go standard library (sync, time) — no external dependencies.
//
// # Token Bucket Algorithm
//
// Each unique client IP gets its own token bucket. The bucket starts full at
// [burst] tokens. One token is consumed per request; tokens are refilled at
// [rps] tokens per second. When the bucket is empty the request is rejected
// with HTTP 429 Too Many Requests.
//
// This is the identical algorithm used by golang.org/x/time/rate but
// implemented here with sync.Mutex + time.Time arithmetic so the codebase
// gains zero new transitive dependencies.
//
// # Cleanup
//
// A background goroutine started by [NewRateLimitMiddleware] evicts IP entries
// that have had no traffic for more than [ttl] (default 10 min) to prevent
// unbounded memory growth under traffic from many unique IPs.
//
// # Usage
//
//	// 5 requests per minute, burst of 10, for auth routes:
//	authLimiter := middleware.NewRateLimitMiddleware(5.0/60, 10)
//	r.With(authLimiter).Post("/api/v1/auth/login", ...)
//
//	// 2 requests per minute for expensive admin scrape trigger:
//	adminLimiter := middleware.NewRateLimitMiddleware(2.0/60, 3)
//	r.Route("/api/v1/admin", func(r chi.Router) {
//	    r.Use(adminLimiter)
//	    ...
//	})
package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// bucket holds token-bucket state for a single client IP.
type bucket struct {
	tokens   float64   // current token count (may be fractional)
	lastSeen time.Time // last request time, used for TTL eviction
	mu       sync.Mutex
}

// allow returns true if the request is permitted (one token consumed).
// It refills tokens proportional to time elapsed since the last call.
func (b *bucket) allow(rps float64, burst float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	// Refill tokens proportional to elapsed time, capped at burst size
	b.tokens = math.Min(burst, b.tokens+elapsed*rps)

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// RateLimiter holds the per-IP bucket map and configuration.
type RateLimiter struct {
	rps   float64
	burst float64

	mu      sync.RWMutex
	buckets map[string]*bucket
}

// NewRateLimitMiddleware returns a chi-compatible middleware function that
// enforces IP-based rate limiting.
//
// Parameters:
//
//	rps   – tokens replenished per second (e.g. 5.0/60 = 5 req/min)
//	burst – maximum tokens in the bucket (= maximum burst request count)
//
// A background cleanup goroutine runs every 10 minutes evicting IPs
// that have been idle for more than 10 minutes.
func NewRateLimitMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	rl := &RateLimiter{
		rps:     rps,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
	}

	// Background cleanup goroutine — evicts stale IP entries
	go rl.cleanup(10 * time.Minute)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			b := rl.getOrCreate(ip)

			if !b.allow(rl.rps, rl.burst) {
				slog.Warn("rate limit exceeded",
					"ip", ip,
					"path", r.URL.Path,
					"method", r.Method,
					"request_id", RequestIDFromContext(r.Context()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Limit", "exceeded")
				// Retry-After: advise client to wait ~1 burst-refill duration
				retryAfter := int(math.Ceil(1.0 / rl.rps))
				w.Header().Set("Retry-After", strings.TrimSpace(http.CanonicalHeaderKey(
					string(rune('0'+retryAfter%10)),
				)))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many requests","code":429,"message":"Rate limit exceeded. Please wait before retrying."}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getOrCreate fetches or initialises the bucket for a given IP.
func (rl *RateLimiter) getOrCreate(ip string) *bucket {
	// Fast path: read lock
	rl.mu.RLock()
	b, ok := rl.buckets[ip]
	rl.mu.RUnlock()
	if ok {
		return b
	}

	// Slow path: write lock — create new bucket starting full
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if b, ok = rl.buckets[ip]; ok {
		return b
	}

	b = &bucket{
		tokens:   rl.burst, // start full
		lastSeen: time.Now(),
	}
	rl.buckets[ip] = b
	return b
}

// cleanup periodically evicts IP buckets that have been idle longer than ttl.
func (rl *RateLimiter) cleanup(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		evicted := 0
		for ip, b := range rl.buckets {
			b.mu.Lock()
			idle := now.Sub(b.lastSeen)
			b.mu.Unlock()

			if idle > ttl {
				delete(rl.buckets, ip)
				evicted++
			}
		}
		rl.mu.Unlock()

		if evicted > 0 {
			slog.Debug("rate limiter: evicted stale IP buckets", "count", evicted)
		}
	}
}

// extractIP extracts the client IP from the request, honouring common
// reverse-proxy headers (X-Real-IP, X-Forwarded-For) before falling back
// to RemoteAddr.
func extractIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can be a comma-separated list; take the first entry
		parts := strings.SplitN(forwarded, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
