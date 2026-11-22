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
	"strings"
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

// rateLimitState holds one bucket per route pattern. A single global bucket
// let a burst on one endpoint (suppression imports, tag edits) starve every
// unrelated mutation — per-endpoint budgets keep the runaway-client
// protection without the starvation.
var rateLimitState = struct {
	sync.Mutex
	buckets map[string]*tokenBucket
}{buckets: map[string]*tokenBucket{}}

// rateLimitProbeMethods is the verb set used to recover the path template of
// a request whose own verb matches no route (405-bound requests resolve to ""
// from mux.Handler).
var rateLimitProbeMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// routeKey resolves the path template this request would route under, without
// the leading verb: the budget for an endpoint is shared across every state-
// changing method so a flooded route limits POST as thoroughly as DELETE.
// Paths matching no route under any verb share one fallback bucket.
func routeKey(mux *http.ServeMux, r *http.Request) string {
	_, pattern := mux.Handler(r)
	if pattern == "" {
		probe := new(http.Request)
		*probe = *r
		for _, method := range rateLimitProbeMethods {
			probe.Method = method
			if _, p := mux.Handler(probe); p != "" {
				pattern = p
				break
			}
		}
	}
	if pattern == "" {
		return "unrouted"
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

// bucketFor returns the token bucket for this request's route. r.Pattern is
// only populated after routing, so the middleware resolves the would-be
// template via routeKey instead.
func bucketFor(mux *http.ServeMux, r *http.Request) *tokenBucket {
	key := routeKey(mux, r)
	rateLimitState.Lock()
	defer rateLimitState.Unlock()
	b, ok := rateLimitState.buckets[key]
	if !ok {
		b = newTokenBucket(rateLimitCapacity, rateLimitRefillPerMinute)
		rateLimitState.buckets[key] = b
	}
	return b
}

// ResetRateLimiter restores full buckets. It exists for tests so rate-limit
// state never leaks between test cases; the swap is mutex-guarded so an
// in-flight request can never observe a torn pointer.
func ResetRateLimiter() {
	rateLimitState.Lock()
	rateLimitState.buckets = map[string]*tokenBucket{}
	rateLimitState.Unlock()
}

// rateLimitMiddleware rejects state-changing requests once the bucket is
// empty, answering 429 with a Retry-After hint. Read-only methods are never
// limited. Requests rejected earlier in the chain (for example by the
// security middleware's origin check) never reach the bucket, and requests
// the router would 404 still count as attempts: limiting happens before any
// handler runs.
func rateLimitMiddleware(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStateChanging(r.Method) {
			if ok, retryAfter := bucketFor(mux, r).take(time.Now()); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				fail(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests; retry shortly.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
