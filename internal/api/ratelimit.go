package api

// A token-bucket rate limit guards the state-changing API surface. The local
// API is served on loopback for a single user, so the limiter exists to catch
// runaway automation (a CI script or a misconfigured client hammering POSTs)
// rather than to ration capacity between tenants: bursts are allowed, and
// sustained mutation pressure above 30 requests per minute starts receiving
// 429 with a Retry-After hint instead of piling work onto the database.

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	// rateLimitCapacity is the burst size: the bucket starts full, so up to
	// this many state-changing requests succeed instantly.
	rateLimitCapacity = 30
	// rateLimitRefillPerMinute is the sustained mutation rate the refill
	// restores; 30/minute matches the burst budget of one per two seconds.
	rateLimitRefillPerMinute = 30
)

// tokenBucket is a classic token bucket: one token per allowed mutating
// request, refilled continuously at refillPerSecond up to capacity. The mutex
// makes take safe from the server's request goroutines.
type tokenBucket struct {
	mu              sync.Mutex
	tokens          float64
	last            time.Time
	capacity        float64
	refillPerSecond float64
}

func newTokenBucket(capacity, perMinute int) *tokenBucket {
	return &tokenBucket{
		tokens:          float64(capacity),
		last:            time.Now(),
		capacity:        float64(capacity),
		refillPerSecond: float64(perMinute) / 60,
	}
}

// take consumes one token, refilling first for the time elapsed since the
// last call. When the bucket is empty it reports how many seconds until the
// next token (rounded up, at least 1) for the Retry-After header.
func (b *tokenBucket) take(now time.Time) (ok bool, retryAfterSeconds int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(b.capacity, b.tokens+elapsed*b.refillPerSecond)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	retry := math.Ceil((1 - b.tokens) / b.refillPerSecond)
	if retry < 1 {
		retry = 1
	}
	return false, int(retry)
}

// apiMutationTokenBucket is the process-wide bucket: production runs exactly
// one server per data directory, so its whole mutating surface shares one
// budget. Tests reset it with ResetRateLimiter.
var apiMutationTokenBucket = newTokenBucket(rateLimitCapacity, rateLimitRefillPerMinute)

// ResetRateLimiter restores a full bucket. It exists for tests so rate-limit
// state never leaks between test cases that share the package-level bucket.
func ResetRateLimiter() { apiMutationTokenBucket = newTokenBucket(rateLimitCapacity, rateLimitRefillPerMinute) }

// rateLimitMiddleware rejects state-changing requests once the bucket is
// empty, answering 429 with a Retry-After hint. Read-only methods are never
// limited. Requests rejected earlier in the chain (for example by the
// security middleware's origin check) never reach the bucket, and requests
// the router would 404 still count as attempts: limiting happens before any
// handler runs.
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStateChanging(r.Method) {
			if ok, retryAfter := apiMutationTokenBucket.take(time.Now()); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				fail(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests; retry shortly.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
