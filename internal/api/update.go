package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"bluntcode/internal/selfupdate"
)

const updateRepository = selfupdate.Repository

// fetchURLBytes performs the outbound HTTP GET for the release check.
// Package-level so tests can stub the network. The caller's context is
// honored — a client disconnecting must cancel the GitHub call, not leave
// it running until the client timeout.
var fetchURLBytes = func(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "BluntCode-Updater")
	request.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// compareSemver orders dotted numeric versions: -1 when a<b, 0 when equal,
// 1 when a>b. A prerelease sorts below its release (1.2.3-beta < 1.2.3) per
// semver §11; build metadata (+…) is ignored.
func compareSemver(a, b string) int {
	as, aPre := splitVersion(a)
	bs, bPre := splitVersion(b)
	for i := 0; i < 3; i++ {
		var an, bn int
		if i < len(as) {
			an = atoiSafe(as[i])
		}
		if i < len(bs) {
			bn = atoiSafe(bs[i])
		}
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}
	// Same numeric version: a prerelease is always less than the release.
	if aPre == bPre {
		return 0
	}
	if aPre == "" {
		return 1
	}
	if bPre == "" {
		return -1
	}
	return strings.Compare(aPre, bPre)
}

// splitVersion separates the dotted numeric core from a -prerelease suffix
// (and drops +build metadata), returning numeric segments and the suffix.
func splitVersion(v string) ([]string, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	return strings.Split(v, "."), pre
}

func atoiSafe(value string) int {
	n := 0
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
}

func (s *Server) offlineEnabled(r *http.Request) bool {
	settings, err := s.db.AppSettings(r.Context())
	if err != nil {
		return false
	}
	return settings.Offline
}

// UpdateCheckResult holds version comparison and release info.
type UpdateCheckResult struct {
	Current      string `json:"current"`
	Latest       string `json:"latest"`
	Available    bool   `json:"available"`
	ReleaseURL   string `json:"release_url"`
	ReleaseNotes string `json:"release_notes"`
	// Unreachable is true when the release check could not run (no network,
	// GitHub unreachable). The answer is still 200: the browser logs every
	// >=400 response as a console error, and "can't check right now" is not
	// an error the UI acts on — it must stay silent.
	Unreachable bool `json:"unreachable"`
}

// CheckUpdateDirect checks GitHub releases for a newer version than currentVersion.
func CheckUpdateDirect(ctx context.Context, currentVersion string) (UpdateCheckResult, error) {
	body, err := fetchURLBytes(ctx, "https://api.github.com/repos/"+updateRepository+"/releases/latest")
	if err != nil {
		return UpdateCheckResult{}, fmt.Errorf("could not reach GitHub releases: %w", err)
	}
	var release releaseInfo
	if err := json.Unmarshal(body, &release); err != nil || release.TagName == "" {
		return UpdateCheckResult{}, fmt.Errorf("github returned an unexpected response")
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	notes := release.Body
	if len(notes) > 600 {
		notes = notes[:600] + "…"
	}
	return UpdateCheckResult{
		Current:      currentVersion,
		Latest:       latest,
		Available:    compareSemver(latest, currentVersion) > 0,
		ReleaseURL:   release.HTMLURL,
		ReleaseNotes: strings.TrimSpace(notes),
	}, nil
}

// updateCheck tells the UI whether a newer release exists. It is only ever
// called on an explicit user action (About page, CLI) or, when the user opted
// in with the update_check setting, once per UI session. Respects offline
// mode: no network traffic happens when the user turned it off.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	if s.offlineEnabled(r) {
		fail(w, http.StatusConflict, "UPDATE_OFFLINE", "Offline mode is enabled; turn it off in Settings to check for updates.")
		return
	}
	res, err := CheckUpdateDirect(r.Context(), s.version)
	if err != nil {
		// Soft-fail with 200 + unavailable instead of a 502 pass-through so an
		// offline machine never accumulates console errors on every load.
		writeJSON(w, http.StatusOK, UpdateCheckResult{Current: s.version, Unreachable: true})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// updateSupported gates in-app updates to Windows. Package-level so tests can
// exercise the staging path on other platforms.
var updateSupported = runtime.GOOS == "windows"

// defaultInstallDir is where install-latest.ps1 puts Blunt Code. The update
// always targets it (not os.Executable()): a portable run from Downloads
// reinstalls into the default dir, matching the one-line installer.
var defaultInstallDir = func() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "BluntCode")
}

// stageRelease is selfupdate.Stage, swappable in tests.
var stageRelease = selfupdate.Stage

// updateApply downloads the newest release zip for its exact tag, verifies
// it (release signature when a key is compiled in, then SHA-256), extracts
// it, and starts a copy of this binary in `update apply-staged` mode. No
// remote script runs and PowerShell is not involved. The helper waits for
// this process to exit — the UI calls /system/stop right after this returns —
// swaps the install directory with rollback, and relaunches the app.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	// Offline is checked first, matching updateCheck: it is the gate the user
	// can act on, and it must hold on every platform.
	if s.offlineEnabled(r) {
		fail(w, http.StatusConflict, "UPDATE_OFFLINE", "Offline mode is enabled; turn it off in Settings to update.")
		return
	}
	if !updateSupported {
		fail(w, http.StatusNotImplemented, "UNSUPPORTED_PLATFORM", "In-app updates are Windows-only.")
		return
	}
	check, err := CheckUpdateDirect(r.Context(), s.version)
	if err != nil {
		fail(w, http.StatusBadGateway, "UPDATE_APPLY_FAILED", "Could not reach GitHub releases: "+err.Error())
		return
	}
	if !check.Available {
		fail(w, http.StatusConflict, "UPDATE_NOT_AVAILABLE", "Blunt Code "+s.version+" is already the latest release.")
		return
	}
	self, err := os.Executable()
	if err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not locate the running Blunt Code executable.")
		return
	}
	dir, err := os.MkdirTemp("", "bluntcode-update-")
	if err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not stage the update.")
		return
	}
	staged, err := stageRelease(r.Context(), "v"+check.Latest, dir, selfupdate.ReleaseSigningKey)
	if err != nil {
		_ = os.RemoveAll(dir)
		fail(w, http.StatusBadGateway, "UPDATE_VERIFY_FAILED", "Update rejected: "+err.Error())
		return
	}
	helper := filepath.Join(dir, "bluntcode-updater.exe")
	if err := selfupdate.CopyFile(self, helper); err != nil {
		_ = os.RemoveAll(dir)
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not stage the updater.")
		return
	}
	command := exec.Command(helper, "update", "apply-staged",
		"--payload", staged.PayloadDir,
		"--install-dir", defaultInstallDir(),
		"--wait-pid", strconv.Itoa(os.Getpid()),
		"--log", filepath.Join(dir, "update.log"),
	)
	command.Dir = dir
	if err := launchUpdateProcess(command); err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not launch the updater.")
		return
	}
	s.log.Info("update staged", "dir", dir, "version", staged.Assets.Version, "signed", staged.Signed)
	writeJSON(w, http.StatusOK, map[string]any{"started": true, "staged_at": dir, "version": staged.Assets.Version, "signed": staged.Signed})
}

// launchUpdateProcess starts the detached updater. Package-level so tests can
// intercept without spawning anything. Detached on purpose: the updater must
// outlive this HTTP request and the app shutdown that follows.
var launchUpdateProcess = selfupdate.StartDetached
