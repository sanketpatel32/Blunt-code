package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bluntcode/internal/database"
	"bluntcode/internal/selfupdate"
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

func TestUpdateCheckSoftFailsWhenUnreachable(t *testing.T) {
	s := testServer(t)
	s.version = "0.6.0"
	original := fetchURLBytes
	fetchURLBytes = func(url string) ([]byte, error) { return nil, fmt.Errorf("dial tcp: connection refused") }
	t.Cleanup(func() { fetchURLBytes = original })
	recorder := httptest.NewRecorder()
	s.updateCheck(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/update/check", nil))
	// An unreachable GitHub is a soft answer (200 + unreachable), not a 502:
	// the browser logs every >=400 response as a console error on every load.
	if recorder.Code != http.StatusOK {
		t.Fatalf("unreachable check status = %d, want 200", recorder.Code)
	}
	var payload struct {
		Available   bool `json:"available"`
		Unreachable bool `json:"unreachable"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Available || !payload.Unreachable {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func stubUpdateApply(t *testing.T) (*exec.Cmd, *string) {
	t.Helper()
	originalSupported, originalStage, originalLaunch, originalDir := updateSupported, stageRelease, launchUpdateProcess, defaultInstallDir
	t.Cleanup(func() {
		updateSupported, stageRelease, launchUpdateProcess, defaultInstallDir = originalSupported, originalStage, originalLaunch, originalDir
	})
	updateSupported = true
	defaultInstallDir = func() string { return `C:\Users\me\AppData\Local\Programs\BluntCode` }
	launched := &exec.Cmd{}
	stagedTag := new(string)
	stageRelease = func(_ context.Context, tag, dir, _ string) (selfupdate.Staged, error) {
		*stagedTag = tag
		payload := filepath.Join(dir, "payload", "app")
		return selfupdate.Staged{Assets: selfupdate.Assets{Tag: tag, Version: strings.TrimPrefix(tag, "v")}, PayloadDir: payload}, nil
	}
	launchUpdateProcess = func(command *exec.Cmd) error { *launched = *command; return nil }
	return launched, stagedTag
}

func TestUpdateApplyStagesVerifiedReleaseWithoutScripts(t *testing.T) {
	s := testServer(t)
	s.version = "0.5.0"
	stubUpdateNetwork(t, `{"tag_name":"v0.6.0","html_url":"u","body":""}`)
	launched, stagedTag := stubUpdateApply(t)

	recorder := httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if *stagedTag != "v0.6.0" {
		t.Fatalf("staged tag = %q, want the exact release tag", *stagedTag)
	}
	var payload struct {
		StagedAt string `json:"staged_at"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(payload.StagedAt) })
	helper := filepath.Join(payload.StagedAt, "bluntcode-updater.exe")
	if launched.Path != helper {
		t.Fatalf("launched %q, want the staged copy of this binary %q", launched.Path, helper)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Fatalf("helper binary not staged: %v", err)
	}
	args := strings.Join(launched.Args, " ")
	for _, want := range []string{"update apply-staged", "--payload " + filepath.Join(payload.StagedAt, "payload", "app"), "--wait-pid " + strconv.Itoa(os.Getpid())} {
		if !strings.Contains(args, want) {
			t.Fatalf("updater args %q missing %q", args, want)
		}
	}
	for _, forbidden := range []string{"powershell", "ExecutionPolicy", ".ps1", "cmd"} {
		if strings.Contains(strings.ToLower(args), strings.ToLower(forbidden)) {
			t.Fatalf("updater still shells out (%q): %s", forbidden, args)
		}
	}
}

func TestUpdateApplyFailsClosed(t *testing.T) {
	s := testServer(t)
	s.version = "0.5.0"
	launched, _ := stubUpdateApply(t)

	original := fetchURLBytes
	fetchURLBytes = func(url string) ([]byte, error) { return nil, fmt.Errorf("rate limited") }
	recorder := httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	fetchURLBytes = original
	if recorder.Code != http.StatusBadGateway || launched.Path != "" {
		t.Fatalf("unreachable releases API must not fall back to another ref: %d %s", recorder.Code, recorder.Body.String())
	}

	stageRelease = func(context.Context, string, string, string) (selfupdate.Staged, error) {
		return selfupdate.Staged{}, fmt.Errorf("release archive SHA-256 mismatch")
	}
	stubUpdateNetwork(t, `{"tag_name":"v0.6.0"}`)
	recorder = httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "UPDATE_VERIFY_FAILED") || launched.Path != "" {
		t.Fatalf("unverified release must not launch the updater: %d %s", recorder.Code, recorder.Body.String())
	}

	stubUpdateNetwork(t, `{"tag_name":"v0.5.0"}`)
	recorder = httptest.NewRecorder()
	s.updateApply(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/update/apply", nil))
	if recorder.Code != http.StatusConflict || launched.Path != "" {
		t.Fatalf("same-version apply must be refused: %d %s", recorder.Code, recorder.Body.String())
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
