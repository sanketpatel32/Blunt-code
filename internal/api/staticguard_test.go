package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStaticGuardBlocksNonLoopbackHosts pins the export that wraps the
// embedded UI: the static file server historically sat outside the API's
// securityMiddleware, so any Host header reached it. Through StaticGuard a
// non-loopback Host is refused with 421 before the file handler runs, while
// loopback traffic passes through with the baseline security headers set.
func TestStaticGuardBlocksNonLoopbackHosts(t *testing.T) {
	served := false
	handler := StaticGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback host must pass, got %d", rec.Code)
	}
	if served != true {
		t.Fatal("loopback request never reached the wrapped handler")
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Content-Security-Policy": "default-src 'self'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	for _, host := range []string{"example.com", "10.0.0.5:8787", "192.168.1.10:8787", "[2001:db8::1]:8787"} {
		served = false
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/", nil)
		req.Host = host
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest {
			t.Errorf("Host %q must be refused with 421, got %d", host, rec.Code)
		}
		if served {
			t.Errorf("Host %q must not reach the wrapped handler", host)
		}
	}
}
