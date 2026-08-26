package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// rateLimitedResponse decodes the 429 envelope.
func decodeRateLimitBody(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("invalid 429 body: %v: %s", err, body)
	}
	return parsed.Error.Code
}

// TestRateLimitBlocksRapidMutations fires 35 rapid DELETEs at a mutating
// route: 404 responses still count as attempts, so the first burst succeeds
// (non-429) until the bucket empties and the remaining requests receive 429
// with the Retry-After hint. Read-only methods are never limited, even when
// the bucket is empty.
func TestRateLimitBlocksRapidMutations(t *testing.T) {
	ResetRateLimiter()
	s := testServer(t)
	target := "http://127.0.0.1/api/v1/scans/00000000-0000-4000-8000-000000000000"

	codes := make([]int, 35)
	for attempt := range codes {
		request := httptest.NewRequest(http.MethodDelete, target, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		codes[attempt] = response.Code
		if response.Code != http.StatusTooManyRequests {
			continue
		}
		if code := decodeRateLimitBody(t, response.Body.Bytes()); code != "RATE_LIMITED" {
			t.Fatalf("429 body must carry RATE_LIMITED, got %q", code)
		}
		retryAfter, err := strconv.Atoi(response.Header().Get("Retry-After"))
		if err != nil || retryAfter < 1 {
			t.Fatalf("429 must carry a Retry-After of at least one second, got %q", response.Header().Get("Retry-After"))
		}
	}
	for attempt, code := range codes[:rateLimitCapacity] {
		if code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was limited before the bucket emptied: %v", attempt+1, codes)
		}
	}
	for attempt, code := range codes[rateLimitCapacity:] {
		if code != http.StatusTooManyRequests {
			t.Fatalf("attempt %d after exhaustion got %d, want 429 (codes %v)", rateLimitCapacity+attempt+1, code, codes)
		}
	}

	// GETs never consume tokens and are never rejected for volume.
	for attempt := 1; attempt <= 10; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/health", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET attempt %d was limited (%d), reads are exempt: %s", attempt, response.Code, response.Body.String())
		}
	}
}

// TestRateLimitCoversEveryStateChangingMethod pins that POST, PUT, PATCH,
// and DELETE all draw from the same budget while GET stays untouched even on
// an exhausted bucket.
func TestRateLimitCoversEveryStateChangingMethod(t *testing.T) {
	ResetRateLimiter()
	s := testServer(t)
	target := "http://127.0.0.1/api/v1/scans/00000000-0000-4000-8000-000000000000"
	// Drain the whole bucket with DELETE attempts to an unknown scan.
	for i := 0; i < rateLimitCapacity; i++ {
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, target, nil))
		if response.Code == http.StatusTooManyRequests {
			t.Fatalf("draw %d was limited although the bucket started full", i+1)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(method, target, nil))
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("%s after exhaustion got %d, want 429", method, response.Code)
		}
		responseGet := httptest.NewRecorder()
		s.Handler().ServeHTTP(responseGet, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/health", nil))
		if responseGet.Code != http.StatusOK {
			t.Fatalf("GET stayed limited (%d) on an exhausted bucket", responseGet.Code)
		}
	}
}

// TestTokenBucketRefillAndRetryMath exercises take directly: a fresh bucket
// serves exactly capacity tokens, then refuses with retry hints until time
// passes, and refills continuously rather than in one lump.
func TestTokenBucketRefillAndRetryMath(t *testing.T) {
	bucket := newTokenBucket(3, 30) // refill 0.5/s -> one token every 2 seconds
	now := bucket.last
	for draw := 0; draw < 3; draw++ {
		if ok, retry := bucket.take(now); !ok || retry != 0 {
			t.Fatalf("draw %d of a fresh bucket: ok=%v retry=%d", draw+1, ok, retry)
		}
	}
	if ok, retry := bucket.take(now); ok || retry != 2 {
		t.Fatalf("empty bucket: ok=%v retry=%d, want false/2 (one token per 2s)", ok, retry)
	}
	// Halfway to the next token it still refuses; after two full seconds the
	// single refilled token is served again.
	if ok, _ := bucket.take(now.Add(time.Second)); ok {
		t.Fatal("refill must not grant half tokens early")
	}
	if ok, retry := bucket.take(now.Add(2 * time.Second)); !ok || retry != 0 {
		t.Fatalf("after one refill interval: ok=%v retry=%d, want true/0", ok, retry)
	}
}
