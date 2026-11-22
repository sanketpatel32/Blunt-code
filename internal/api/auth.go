package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// TokenFragmentKey is the URL fragment key the launcher passes the token in
// (http://127.0.0.1:8787/#token=...). Fragments never reach the server, logs,
// or Referer headers. The UI keeps it in localStorage, which, unlike cookies,
// is isolated per origin including the port, so no other localhost server the
// browser visits is ever sent it.
const TokenFragmentKey = "token"

// NewToken returns a fresh 256-bit API token.
// NewToken mints a credential; the error is returned, not panic()ed, because
// tokens are minted on request paths (session tickets), not only at startup.
// Init-time callers that cannot propagate use mustToken.
func NewToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("crypto/rand unavailable: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// mustToken is for process bootstrap only: a token at startup is mandatory,
// so failure there is legitimately fatal.
func mustToken() string {
	token, err := NewToken()
	if err != nil {
		panic("bluntcode: " + err.Error())
	}
	return token
}

// Token is the credential every /api/ request except GET /health must carry,
// as `Authorization: Bearer <token>`. Plain navigations that cannot set a
// header (EventSource, downloads) use a short-lived ticket instead.
func (s *Server) Token() string { return s.token }

// SetToken replaces the generated token, e.g. so the host process can share
// one token between the HTTP server and its on-disk token file.
func (s *Server) SetToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("bluntcode: empty API token")
	}
	s.token = token
	return nil
}

// authenticate reports whether r carries the server token as a bearer
// header.
func (s *Server) authenticate(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}
	scheme, value, ok := strings.Cut(header, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && s.tokenMatches(strings.TrimSpace(value))
}

func (s *Server) tokenMatches(candidate string) bool {
	if s.token == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(s.token)) == 1
}

// publicRoute lists the API routes reachable without a credential: the
// liveness probe only. Everything else, reads included, can expose source or
// workspace layout.
func publicRoute(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/api/"+APIVersion+"/health"
}

// apiGuard enforces, in order: same-origin browser context (CSRF and
// cross-port reads), the API token, and JSON-only request bodies.
func (s *Server) apiGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browser, sameOrigin := browserOrigin(r)
		if browser && !sameOrigin {
			fail(w, http.StatusForbidden, "CROSS_ORIGIN_FORBIDDEN", "Requests must come from the local Blunt Code origin.")
			return
		}
		if publicRoute(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.authenticate(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="bluntcode"`)
			fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "This Blunt Code session is not signed in. Reopen Blunt Code from its launcher, or open the sign-in URL it printed.")
			return
		}
		// A text/plain or form body is what a no-preflight cross-origin
		// write looks like; only JSON is ever a legitimate body here.
		if isStateChanging(r.Method) && hasBody(r) && !jsonContentType(r) {
			fail(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Request bodies must be sent as application/json.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// browserOrigin reports whether the request carries browser origin metadata
// and, if so, whether it is exactly this server's origin (scheme, host, and
// port). Sec-Fetch-Site is authoritative when present: "same-origin" is our
// own page and "none" is the user typing or bookmarking a URL. Without it,
// Origin must equal http://<Host>; http://localhost:3000 calling
// localhost:8787 is cross-origin even though it is the same site.
func browserOrigin(r *http.Request) (browser, sameOrigin bool) {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return true, site == "same-origin" || site == "none"
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false, false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Host == "" || u.Path != "" {
		return true, false
	}
	return true, strings.EqualFold(u.Host, r.Host) && loopbackHost(u.Host)
}

func hasBody(r *http.Request) bool {
	return r.ContentLength > 0 || r.ContentLength == -1 && r.Body != nil && r.Body != http.NoBody
}

func jsonContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

// session reports whether the caller is signed in; the guard has already
// answered 401 otherwise.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
