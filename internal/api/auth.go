package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// SessionCookie carries the per-launch API token for the embedded UI. It is
// HttpOnly so page scripts never hold the token, SameSite=Strict so no
// cross-site navigation carries it, and scoped to /api/ so static assets
// never see it. SameSite alone does not stop other localhost ports (ports
// are the same site), which is why every API request is also checked for a
// same-origin Sec-Fetch-Site / Origin.
const SessionCookie = "bluntcode_session"

// TokenFragmentKey is the URL fragment key the launcher passes the token in
// (http://127.0.0.1:8787/#token=...). Fragments never reach the server, logs,
// or Referer headers; the UI trades it for the session cookie on load.
const TokenFragmentKey = "token"

// NewToken returns a fresh 256-bit API token.
func NewToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("bluntcode: crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Token is the credential every /api/ request except GET /health must carry,
// as `Authorization: Bearer <token>` or the session cookie.
func (s *Server) Token() string { return s.token }

// SetToken replaces the generated token, e.g. so the host process can share
// one token between the HTTP server and its on-disk token file.
func (s *Server) SetToken(token string) {
	if strings.TrimSpace(token) == "" {
		panic("bluntcode: empty API token")
	}
	s.token = token
}

type credential int

const (
	credentialNone credential = iota
	credentialBearer
	credentialCookie
)

// authenticate reports which credential, if any, matches the server token.
// A bearer header is checked first; a wrong bearer is never rescued by a
// cookie.
func (s *Server) authenticate(r *http.Request) credential {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, value, ok := strings.Cut(header, " ")
		if ok && strings.EqualFold(scheme, "Bearer") && s.tokenMatches(strings.TrimSpace(value)) {
			return credentialBearer
		}
		return credentialNone
	}
	if cookie, err := r.Cookie(SessionCookie); err == nil && s.tokenMatches(cookie.Value) {
		return credentialCookie
	}
	return credentialNone
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
		cred := s.authenticate(r)
		if cred == credentialNone {
			w.Header().Set("WWW-Authenticate", `Bearer realm="bluntcode"`)
			fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "This Blunt Code session is not signed in. Reopen Blunt Code from its launcher, or open the sign-in URL it printed.")
			return
		}
		// Cookies ride along automatically, so a cookie-authenticated write
		// must prove it came from our own page. Browsers always attach
		// Origin or Sec-Fetch-Site to such writes; their absence means a
		// client we cannot attribute.
		if cred == credentialCookie && isStateChanging(r.Method) && !browser {
			fail(w, http.StatusForbidden, "CROSS_ORIGIN_FORBIDDEN", "State-changing requests must come from the local Blunt Code origin.")
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

// session trades a bearer token for the HttpOnly session cookie (POST), or
// reports whether the caller is signed in (GET; the guard already answered
// 401 otherwise).
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if s.authenticate(r) != credentialBearer {
			fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in with the token from the Blunt Code launcher.")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: s.token, Path: "/api/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	w.WriteHeader(http.StatusNoContent)
}
