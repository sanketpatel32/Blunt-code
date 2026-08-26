package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bluntcode/internal/database"
)

func TestCompareSemverOrdersTriples(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.6.0", "0.5.0", 1},
		{"0.5.0", "0.6.0", -1},
		{"0.6.0", "0.6.0", 0},
		{"v1.0.0", "1.0.0", 0},
		{"1.0", "1.0.0", 0},
		{"10.0.0", "9.99.99", 1},
	}
	for _, tc := range cases {
		if got := compareSemver(tc.a, tc.b); got != tc.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func stubUpdateNetwork(t *testing.T, payload string) *[]byte {
	t.Helper()
	calls := &[]byte{}
	original := fetchURLBytes
	fetchURLBytes = func(url string) ([]byte, error) {
		*calls = []byte(url)
		return []byte(payload), nil
	}
	t.Cleanup(func() { fetchURLBytes = original })
	return calls
}

func TestUpdateCheckReportsNewerRelease(t *testing.T) {
	s := testServer(t)
	s.version = "0.5.0"
	stubUpdateNetwork(t, `{"tag_name":"v0.6.0","html_url":"https://github.com/sanketpatel32/Blunt-code/releases/tag/v0.6.0","body":"Notes here"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/update/check", nil)
	s.updateCheck(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Current   string `json:"current"`
		Latest    string `json:"latest"`
		Available bool   `json:"available"`
		Notes     string `json:"release_notes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Current != "0.5.0" || payload.Latest != "0.6.0" || !payload.Available {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Notes != "Notes here" {
		t.Fatalf("notes = %q", payload.Notes)
	}
}

func TestUpdateCheckUpToDateAndOffline(t *testing.T) {
	s := testServer(t)
	s.version = "0.6.0"
	stubUpdateNetwork(t, `{"tag_name":"v0.6.0","html_url":"u","body":""}`)
	recorder := httptest.NewRecorder()
	s.updateCheck(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/check", nil))
	var payload struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Available {
		t.Fatal("same version must not report an update")
	}

	ctx := context.Background()
	if err := s.db.SaveAppSettings(ctx, database.AppSettings{Offline: true}); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	s.updateCheck(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/check", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("offline check status = %d, want 409", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "UPDATE_OFFLINE") {
		t.Fatalf("offline error code missing: %s", recorder.Body.String())
	}
}

func TestUpdateApplyStagesInstallerAndLaunchesDetached(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("apply flow is Windows-specific")
	}
	s := testServer(t)
	s.version = "0.5.0"
	urlsSeen := stubUpdateNetwork(t, "# fake installer script\r\nparam()\r\n")
	var launched *exec.Cmd
	originalLaunch := launchUpdateProcess
	launchUpdateProcess = func(command *exec.Cmd) error { launched = command; return nil }
	t.Cleanup(func() { launchUpdateProcess = originalLaunch })

	recorder := httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if launched == nil {
		t.Fatal("updater process was not started")
	}
	// The launcher must point at the staged script and run it silently with a
	// close-wait so the app can shut down first.
	var payload struct {
		StagedAt string `json:"staged_at"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(payload.StagedAt, "install-latest.ps1"))
	if err != nil || !strings.Contains(string(script), "fake installer") {
		t.Fatalf("staged installer missing or wrong: %v", err)
	}
	launcher, err := os.ReadFile(filepath.Join(payload.StagedAt, "run-update.cmd"))
	if err != nil || !strings.Contains(string(launcher), "-Silent -WaitForCloseSeconds 60") {
		t.Fatalf("staged launcher missing or wrong: %v", err)
	}
	if len(*urlsSeen) == 0 || !strings.Contains(string(*urlsSeen), "raw.githubusercontent.com") {
		t.Fatalf("installer script was not fetched from the repo: %s", string(*urlsSeen))
	}
}

func TestUpdateApplyBlockedWhenOffline(t *testing.T) {
	s := testServer(t)
	stubUpdateNetwork(t, `{"tag_name":"v9.9.9"}`)
	ctx := context.Background()
	if err := s.db.SaveAppSettings(ctx, database.AppSettings{Offline: true}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}
