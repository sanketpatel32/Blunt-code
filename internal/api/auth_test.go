package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
	// A wrong bearer is not rescued by a valid cookie.
	request := guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/workspaces", "", map[string]string{"Authorization": "Bearer wrong"})
	request.AddCookie(&http.Cookie{Name: SessionCookie, Value: s.Token()})
	if got := serve(s, request).Code; got != http.StatusUnauthorized {
		t.Fatalf("wrong bearer with a valid cookie must be refused, got %d", got)
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

func TestSessionExchangeSetsStrictHttpOnlyCookie(t *testing.T) {
	s := testServer(t)
	headers := map[string]string{"Authorization": "Bearer " + s.Token(), "Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:8787"}
	response := serve(s, guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/session", "", headers))
	if response.Code != http.StatusNoContent {
		t.Fatalf("session exchange: %d %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want one cookie, got %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookie || cookie.Value != s.Token() || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/" {
		t.Fatalf("cookie must be HttpOnly, SameSite=Strict, Path=/api/: %#v", cookie)
	}

	// The cookie alone authenticates the browser's reads (EventSource,
	// download links) and same-origin writes.
	read := guardRequest(http.MethodGet, "http://127.0.0.1:8787/api/v1/session", "", nil)
	read.AddCookie(cookie)
	if got := serve(s, read).Code; got != http.StatusNoContent {
		t.Fatalf("cookie session check: %d", got)
	}
	write := guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":true}`, map[string]string{"Content-Type": "application/json", "Origin": "http://127.0.0.1:8787"})
	write.AddCookie(cookie)
	if got := serve(s, write); got.Code != http.StatusOK {
		t.Fatalf("same-origin cookie write must pass: %d %s", got.Code, got.Body.String())
	}
	// Without Origin or Sec-Fetch-Site a cookie write cannot be attributed to
	// our page and is refused; bearer clients do not need them.
	write = guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":true}`, map[string]string{"Content-Type": "application/json"})
	write.AddCookie(cookie)
	if got := serve(s, write).Code; got != http.StatusForbidden {
		t.Fatalf("cookie write without origin metadata must be refused, got %d", got)
	}

	// A cookie cannot mint another cookie; only the launcher token can.
	again := guardRequest(http.MethodPost, "http://127.0.0.1:8787/api/v1/session", "", map[string]string{"Origin": "http://127.0.0.1:8787"})
	again.AddCookie(cookie)
	if got := serve(s, again).Code; got != http.StatusUnauthorized {
		t.Fatalf("cookie-only session exchange must be refused, got %d", got)
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
		for _, withCookie := range []bool{false, true} {
			headers := map[string]string{"Content-Type": "application/json"}
			for key, value := range tc.headers {
				headers[key] = value
			}
			if !withCookie {
				headers["Authorization"] = bearer
			}
			request := guardRequest(tc.method, "http://127.0.0.1:8787/api/v1/settings", "", headers)
			if tc.method == http.MethodPost {
				request = guardRequest(http.MethodPatch, "http://127.0.0.1:8787/api/v1/settings", `{"offline":false}`, headers)
			}
			if withCookie {
				request.AddCookie(&http.Cookie{Name: SessionCookie, Value: s.Token()})
			}
			if got := serve(s, request); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "CROSS_ORIGIN_FORBIDDEN") {
				t.Errorf("%s (cookie=%v): got %d %s", tc.name, withCookie, got.Code, got.Body.String())
			}
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
