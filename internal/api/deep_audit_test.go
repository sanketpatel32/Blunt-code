package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func send(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

func decodeError(t *testing.T, response *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("error body is not JSON: %q", response.Body.String())
	}
	return envelope
}

// TestCreateWorkspacePathErrorsArePlainLanguage pins the add-workspace error
// contract: every path mistake is a 400 INVALID_PATH aimed at root_path with
// a sentence a person can act on, never a raw Go error string.
func TestCreateWorkspacePathErrorsArePlainLanguage(t *testing.T) {
	s := testServer(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, path, want string }{
		{"empty", "   ", "Enter the folder"},
		{"relative", "src/app", "full folder path"},
		{"missing", filepath.Join(dir, "nope"), "does not exist"},
		{"file", file, "is a file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"root_path": tc.path})
			response := send(t, s, http.MethodPost, "/api/v1/workspaces", string(body))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			envelope := decodeError(t, response)
			if envelope.Error.Code != "INVALID_PATH" || envelope.Error.Field != "root_path" {
				t.Fatalf("envelope %+v", envelope.Error)
			}
			if !strings.Contains(envelope.Error.Message, tc.want) {
				t.Fatalf("message %q, want it to contain %q", envelope.Error.Message, tc.want)
			}
			for _, raw := range []string{"lstat", "stat ", "no such file", "CreateFile", "workspace root"} {
				if strings.Contains(envelope.Error.Message, raw) {
					t.Fatalf("message leaks a raw error: %q", envelope.Error.Message)
				}
			}
		})
	}
}

// TestCreateWorkspaceAcceptsQuotedPaths covers the Windows "Copy as path"
// habit of wrapping the folder in double quotes.
func TestCreateWorkspaceAcceptsQuotedPaths(t *testing.T) {
	s := testServer(t)
	dir := t.TempDir()
	body, _ := json.Marshal(map[string]string{"root_path": `"` + dir + `"`})
	response := send(t, s, http.MethodPost, "/api/v1/workspaces", string(body))
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateWorkspaceBadJSONIsInvalidJSON(t *testing.T) {
	s := testServer(t)
	response := send(t, s, http.MethodPost, "/api/v1/workspaces", "{not json")
	if response.Code != http.StatusBadRequest || decodeError(t, response).Error.Code != "INVALID_JSON" {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
}

func TestSettingsBadJSONIsInvalidJSON(t *testing.T) {
	s := testServer(t)
	response := send(t, s, http.MethodPatch, "/api/v1/settings", "{oops")
	if response.Code != http.StatusBadRequest || decodeError(t, response).Error.Code != "INVALID_JSON" {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	response = send(t, s, http.MethodPatch, "/api/v1/settings", "{}")
	if response.Code != http.StatusBadRequest || decodeError(t, response).Error.Code != "INVALID_SETTINGS" {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
}

// TestUnknownAPIRoutesAnswerJSON: mux 404/405s for /api/ paths use the same
// error envelope as handlers, so the UI can always read error.code.
func TestUnknownAPIRoutesAnswerJSON(t *testing.T) {
	s := testServer(t)
	response := get(t, s, http.MethodGet, "/api/v1/does-not-exist")
	if response.Code != http.StatusNotFound || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status %d type %q", response.Code, response.Header().Get("Content-Type"))
	}
	if decodeError(t, response).Error.Code != "NOT_FOUND" {
		t.Fatalf("body %s", response.Body.String())
	}
	response = get(t, s, http.MethodDelete, "/api/v1/health")
	if response.Code != http.StatusMethodNotAllowed || decodeError(t, response).Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Allow") == "" {
		t.Fatal("405 must keep the Allow header")
	}
}
