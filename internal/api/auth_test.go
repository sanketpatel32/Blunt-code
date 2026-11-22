package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func guardRequest(method, target, body string, headers map[string]string) *http.Request {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, target, nil)
	} else {
		request = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return request
}

func serve(s *Server, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

func TestAPIRequiresTokenOnEveryRouteButHealth(t *testing.T) {
	s := testServer(t)
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/health", "", nil)).Code; got != http.StatusOK {
		t.Fatalf("health must stay public for liveness probes, got %d", got)
	}
	for _, target := range []string{"/api/v1/meta", "/api/v1/workspaces", "/api/v1/settings", "/api/v1/stats", "/api/v1/session"} {
		response := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787"+target, "", nil))
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "UNAUTHORIZED") {
			t.Errorf("GET %s without a token: got %d %s", target, response.Code, response.Body.String())
		}
		if response.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("GET %s: 401 must name the bearer scheme", target)
		}
	}
	for _, header := range []string{"Bearer wrong", "Bearer ", "Basic " + s.Token(), s.Token()} {
		response := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/workspaces", "", map[string]string{"Authorization": header}))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q must be refused, got %d", header, response.Code)
		}
	}
	// A wrong bearer is not rescued by a valid ticket.
	ticket := mintTicket(t, s, "/api/v1/workspaces")
	request := guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/workspaces?ticket="+ticket, "", map[string]string{"Authorization": "Bearer wrong"})
	if got := serve(s, request).Code; got != http.StatusUnauthorized {
		t.Fatalf("wrong bearer with a valid ticket must be refused, got %d", got)
	}
	ok := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/workspaces", "", map[string]string{"Authorization": "Bearer " + s.Token()}))
	if ok.Code != http.StatusOK {
		t.Fatalf("valid bearer must pass: %d %s", ok.Code, ok.Body.String())
	}
}

func TestUnauthenticatedWritesNeverReachHandlers(t *testing.T) {
	s := testServer(t)
	stopped := false
	s.SetShutdown(func() { stopped = true })
	response := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/system/stop", "", nil))
	if response.Code != http.StatusUnauthorized || stopped {
		t.Fatalf("unauthenticated stop must be refused: %d stopped=%v", response.Code, stopped)
	}
	response = serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/workspaces", `{"root_path":`+quote(t.TempDir())+`}`, map[string]string{"Content-Type": "application/json"}))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated workspace registration must be refused: %d", response.Code)
	}
}

func TestTokensAreRandomPerServer(t *testing.T) {
	a, b := testServer(t), testServer(t)
	if a.Token() == "" || a.Token() == b.Token() || len(a.Token()) < 40 {
		t.Fatalf("tokens must be long and unique per launch: %q %q", a.Token(), b.Token())
	}
}

func mintTicket(t *testing.T, s *Server, path string) string {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + s.Token(), "Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"}
	response := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/session/tickets", `{"path":"`+path+`"}`, headers))
	if response.Code != http.StatusOK {
		t.Fatalf("mint ticket for %s: %d %s", path, response.Code, response.Body.String())
	}
	var body struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Ticket) < 43 || body.ExpiresIn <= 0 {
		t.Fatalf("ticket response %s: %v", response.Body.String(), err)
	}
	return body.Ticket
}

func TestNoCookieCarriesTheToken(t *testing.T) {
	s := testServer(t)
	headers := map[string]string{"Authorization": "Bearer " + s.Token(), "Sec-Fetch-Site": "same-origin"}
	response := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/session", "", headers))
	if response.Code != http.StatusNoContent {
		t.Fatalf("session check: %d %s", response.Code, response.Body.String())
	}
	// Cookies are shared by every port on a host, so any localhost server the
	// browser visits would receive one; the token must never ride in one.
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("no cookie may be set, got %#v", cookies)
	}
	request := guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/settings", "", nil)
	request.AddCookie(&http.Cookie{Name: "bluntcode_session", Value: s.Token()})
	if got := serve(s, request).Code; got != http.StatusUnauthorized {
		t.Fatalf("a cookie must not authenticate, got %d", got)
	}
}

func TestTicketsAreSingleUsePathBoundGetOnly(t *testing.T) {
	s := testServer(t)
	path := "/api/v1/settings"
	ticket := mintTicket(t, s, path)
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787"+path+"?ticket="+ticket, "", map[string]string{"Sec-Fetch-Site": "same-origin"})); got.Code != http.StatusOK {
		t.Fatalf("ticket GET: %d %s", got.Code, got.Body.String())
	}
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787"+path+"?ticket="+ticket, "", nil)).Code; got != http.StatusUnauthorized {
		t.Fatalf("a used ticket must be refused, got %d", got)
	}
	other := mintTicket(t, s, path)
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/workspaces?ticket="+other, "", nil)).Code; got != http.StatusUnauthorized {
		t.Fatalf("a ticket for another path must be refused, got %d", got)
	}
	// A failed redemption still burns the ticket.
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787"+path+"?ticket="+other, "", nil)).Code; got != http.StatusUnauthorized {
		t.Fatalf("a burned ticket must be refused, got %d", got)
	}
	write := mintTicket(t, s, path)
	if got := serve(s, guardRequest(http.MethodPatch, "http://127.0.0.1:8787"+path+"?ticket="+write, `{"offline":true}`, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8787"})).Code; got != http.StatusUnauthorized {
		t.Fatalf("tickets must not authorize writes, got %d", got)
	}
	expired := mintTicket(t, s, path)
	s.tickets.mu.Lock()
	entry := s.tickets.tickets[expired]
	entry.expires = time.Now().Add(-time.Second)
	s.tickets.tickets[expired] = entry
	s.tickets.mu.Unlock()
	if got := serve(s, guardRequest(http.MethodGet, "http://127.0.0.1:8787"+path+"?ticket="+expired, "", nil)).Code; got != http.StatusUnauthorized {
		t.Fatalf("an expired ticket must be refused, got %d", got)
	}
}

func TestTicketMintingRequiresTheTokenAndAnAPIPath(t *testing.T) {
	s := testServer(t)
	ticket := mintTicket(t, s, "/api/v1/settings")
	// A ticket cannot mint another (it is GET-only and never a bearer).
	if got := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/session/tickets?ticket="+ticket, `{"path":"/api/v1/settings"}`, map[string]string{"Content-Type": "application/json"})).Code; got != http.StatusUnauthorized {
		t.Fatalf("ticket-authenticated mint must be refused, got %d", got)
	}
	headers := map[string]string{"Authorization": "Bearer " + s.Token(), "Content-Type": "application/json"}
	for _, path := range []string{"/", "/index.html", "/api/v2/settings", "/api/v1/session", "/api/v1/session/tickets", "/api/v1/settings?x=1"} {
		if got := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/session/tickets", `{"path":"`+path+`"}`, headers)).Code; got != http.StatusBadRequest {
			t.Errorf("ticket for %q: want 400, got %d", path, got)
		}
	}
}

func TestTicketQueryIsStrippedBeforeHandlers(t *testing.T) {
	s := testServer(t)
	path := "/api/v1/workspaces"
	ticket := mintTicket(t, s, path)
	var seen string
	handler := s.apiGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r.URL.RawQuery }))
	handler.ServeHTTP(httptest.NewRecorder(), guardRequest(http.MethodGet, "http://127.0.0.1:8787"+path+"?limit=5&ticket="+ticket, "", nil))
	if seen != "limit=5" {
		t.Fatalf("handler saw query %q", seen)
	}
}

func TestRejectsOtherLocalOriginsEvenWithCredentials(t *testing.T) {
	s := testServer(t)
	bearer := "Bearer " + s.Token()
	cases := []struct {
		name    string
		method  string
		headers map[string]string
	}{
		{"other localhost port", http.MethodPost, map[string]string{"Origin": "http://localhost:3000"}},
		{"other loopback port", http.MethodPost, map[string]string{"Origin": "http://127.0.0.1:3000"}},
		{"localhost vs 127.0.0.1", http.MethodPost, map[string]string{"Origin": "http://localhost:8787"}},
		{"https scheme", http.MethodPost, map[string]string{"Origin": "https://127.0.0.1:8787"}},
		{"opaque origin", http.MethodPost, map[string]string{"Origin": "null"}},
		{"foreign site", http.MethodPost, map[string]string{"Origin": "https://example.com"}},
		{"same-site fetch", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://127.0.0.1:8787"}},
		{"cross-site fetch", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"cross-port read", http.MethodGet, map[string]string{"Sec-Fetch-Site": "same-site"}},
		{"cross-origin CORS read", http.MethodGet, map[string]string{"Origin": "http://localhost:5500"}},
	}
	for _, tc := range cases {
		headers := map[string]string{"Content-Type": "application/json", "Authorization": bearer}
		for key, value := range tc.headers {
			headers[key] = value
		}
		request := guardRequest(tc.method, "http://127.0.0.1:8787/api/v1/settings", "", headers)
		if tc.method == http.MethodPost {
			request = guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":false}`, headers)
		}
		if got := serve(s, request); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "CROSS_ORIGIN_FORBIDDEN") {
			t.Errorf("%s: got %d %s", tc.name, got.Code, got.Body.String())
		}
	}
	// Our own origin passes, by Sec-Fetch-Site or by exact Origin match.
	for _, headers := range []map[string]string{
		{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:8787"},
		{"Origin": "http://127.0.0.1:8787"},
	} {
		headers["Authorization"] = bearer
		headers["Content-Type"] = "application/json"
		if got := serve(s, guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":false}`, headers)); got.Code != http.StatusOK {
			t.Errorf("same-origin write %v: got %d %s", headers, got.Code, got.Body.String())
		}
	}
}

func TestRejectsNonJSONBodies(t *testing.T) {
	s := testServer(t)
	for _, contentType := range []string{"", "text/plain", "text/plain;charset=UTF-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		headers := map[string]string{"Authorization": "Bearer " + s.Token()}
		if contentType != "" {
			headers["Content-Type"] = contentType
		}
		got := serve(s, guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":true}`, headers))
		if got.Code != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: got %d %s", contentType, got.Code, got.Body.String())
		}
	}
	got := serve(s, guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":true}`, map[string]string{"Authorization": "Bearer " + s.Token(), "Content-Type": "application/json; charset=utf-8"}))
	if got.Code != http.StatusOK {
		t.Fatalf("JSON with charset must pass: %d %s", got.Code, got.Body.String())
	}
	// Bodiless actions need no Content-Type.
	got = serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/system/stop", "", map[string]string{"Authorization": "Bearer " + s.Token()}))
	if got.Code == http.StatusUnsupportedMediaType {
		t.Fatalf("bodiless POST must not require a Content-Type: %d", got.Code)
	}
}

func TestRefusesSensitiveWorkspaceRoots(t *testing.T) {
	s := testServer(t)
	roots := []string{string(os.PathSeparator)}
	if home, err := os.UserHomeDir(); err == nil {
		if info, err := os.Stat(home); err == nil && info.IsDir() {
			roots = append(roots, home)
		}
	}
	windows := t.TempDir()
	t.Setenv("WINDIR", windows)
	if err := os.Mkdir(windows+string(os.PathSeparator)+"System32", 0o755); err != nil {
		t.Fatal(err)
	}
	roots = append(roots, windows, windows+string(os.PathSeparator)+"System32")
	for _, root := range roots {
		got := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/workspaces", `{"root_path":`+quote(root)+`}`, map[string]string{"Authorization": "Bearer " + s.Token(), "Content-Type": "application/json"}))
		if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "INVALID_PATH") {
			t.Errorf("root %q must be refused: %d %s", root, got.Code, got.Body.String())
		}
	}
	project := t.TempDir()
	got := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/workspaces", `{"root_path":`+quote(project)+`}`, map[string]string{"Authorization": "Bearer " + s.Token(), "Content-Type": "application/json"}))
	if got.Code != http.StatusCreated {
		t.Fatalf("an ordinary project folder must still register: %d %s", got.Code, got.Body.String())
	}
}
