package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenFragmentKey is the URL fragment key the launcher passes the token in
// (http://127.0.0.1:8787/#token=...). Fragments never reach the server, logs,
// or Referer headers. The UI keeps it in localStorage, which, unlike cookies,
// is isolated per origin including the port, so no other localhost server the
// browser visits is ever sent it.
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
// as `Authorization: Bearer <token>`. Plain navigations that cannot set a
// header (EventSource, downloads) use a short-lived ticket instead.
func (s *Server) Token() string { return s.token }

// SetToken replaces the generated token, e.g. so the host process can share
// one token between the HTTP server and its on-disk token file.
func (s *Server) SetToken(token string) {
	if strings.TrimSpace(token) == "" {
		panic("bluntcode: empty API token")
	}
	s.token = token
}

// TicketParam is the query parameter carrying a ticket on GET navigations.
const TicketParam = "ticket"

const ticketTTL = time.Minute

// ticketStore holds single-use tickets, each valid for one GET of one API
// path for ticketTTL. A ticket in a URL can end up in history or a Referer,
// so it must be worthless once used or expired, and useless for any other
// path.
type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]ticket
}

type ticket struct {
	path    string
	expires time.Time
}

func (t *ticketStore) mint(path string, now time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tickets == nil {
		t.tickets = map[string]ticket{}
	}
	for id, existing := range t.tickets {
		if !now.Before(existing.expires) {
			delete(t.tickets, id)
		}
	}
	id := NewToken()
	t.tickets[id] = ticket{path: path, expires: now.Add(ticketTTL)}
	return id
}

func (t *ticketStore) redeem(id, path string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	found, ok := t.tickets[id]
	if !ok {
		return false
	}
	delete(t.tickets, id)
	return found.path == path && now.Before(found.expires)
}

// authenticate reports whether r carries the server token as a bearer
// header. A present but wrong Authorization header is never rescued by a
// ticket.
func (s *Server) authenticate(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}
	scheme, value, ok := strings.Cut(header, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && s.tokenMatches(strings.TrimSpace(value))
}

// redeemTicket authenticates a header-less GET by its single-use ticket and
// returns the request with the ticket stripped from the query.
func (s *Server) redeemTicket(r *http.Request) (*http.Request, bool) {
	if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
		return r, false
	}
	query := r.URL.Query()
	id := query.Get(TicketParam)
	if id == "" || !s.tickets.redeem(id, r.URL.Path, time.Now()) {
		return r, false
	}
	query.Del(TicketParam)
	stripped := r.Clone(r.Context())
	stripped.URL.RawQuery = query.Encode()
	stripped.RequestURI = stripped.URL.RequestURI()
	return stripped, true
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
			stripped, ok := s.redeemTicket(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="bluntcode"`)
				fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "This Blunt Code session is not signed in. Reopen Blunt Code from its launcher, or open the sign-in URL it printed.")
				return
			}
			r = stripped
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

// sessionTicket mints a single-use ticket for one GET of one API path, for
// navigations that cannot send the Authorization header. Only the bearer
// token can mint; a ticket cannot mint another.
func (s *Server) sessionTicket(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		fail(w, http.StatusUnauthorized, "UNAUTHORIZED", "Tickets require the API token.")
		return
	}
	var input struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&input); err != nil {
		fail(w, http.StatusBadRequest, "INVALID_REQUEST", "Send {\"path\": \"/api/v1/...\"}.")
		return
	}
	prefix := "/api/" + APIVersion + "/"
	if !strings.HasPrefix(input.Path, prefix) || strings.ContainsAny(input.Path, "?#") || strings.HasPrefix(input.Path, prefix+"session") {
		fail(w, http.StatusBadRequest, "INVALID_REQUEST", "Tickets are issued for a single /api/"+APIVersion+"/ path.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": s.tickets.mint(input.Path, time.Now()), "expires_in": int(ticketTTL.Seconds())})
}
