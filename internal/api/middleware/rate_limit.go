package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type clientRateState struct {
	count    int
	resetAt  time.Time
	lastSeen time.Time
}

// RateLimiter applies a simple in-memory fixed-window rate limit per client.
type RateLimiter struct {
	requests int
	window   time.Duration

	mu      sync.Mutex
	clients map[string]clientRateState
}

// NewRateLimiter creates a new limiter with a request budget per window.
func NewRateLimiter(requests int, window time.Duration) *RateLimiter {
	if requests <= 0 || window <= 0 {
		return nil
	}

	return &RateLimiter{
		requests: requests,
		window:   window,
		clients:  make(map[string]clientRateState),
	}
}

// Middleware enforces the rate limit and returns HTTP 429 when exceeded.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := l.allow(clientKey(r), time.Now())
		if !allowed {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", retryAfter.String())
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":      "rate limit exceeded",
				"code":       http.StatusTooManyRequests,
				"retryAfter": retryAfter.String(),
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prune(now)

	state := l.clients[key]
	if state.resetAt.IsZero() || !now.Before(state.resetAt) {
		state = clientRateState{
			count:    0,
			resetAt:  now.Add(l.window),
			lastSeen: now,
		}
	}

	state.lastSeen = now
	if state.count >= l.requests {
		l.clients[key] = state
		retryAfter := time.Until(state.resetAt).Round(time.Second)
		if retryAfter <= 0 {
			retryAfter = time.Second
		}
		return false, retryAfter
	}

	state.count++
	l.clients[key] = state
	return true, 0
}

func (l *RateLimiter) prune(now time.Time) {
	if len(l.clients) < 1024 {
		return
	}

	cutoff := now.Add(-2 * l.window)
	for key, state := range l.clients {
		if state.lastSeen.Before(cutoff) {
			delete(l.clients, key)
		}
	}
}

func clientKey(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
