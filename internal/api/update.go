package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const updateRepository = "sanketpatel32/Blunt-code"

// fetchURLBytes performs the outbound HTTP GET for update checks and installer
// downloads. Package-level so tests can stub the network.
var fetchURLBytes = func(url string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
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
// 1 when a>b. Non-numeric segments fall back to string order so a stray
// suffix never panics.
func compareSemver(a, b string) int {
	as := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	bs := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
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
	return 0
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

// updateCheck tells the UI whether a newer release exists. Respects offline
// mode: the promise is local-first, so no network traffic happens when the
// user turned it off.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	if s.offlineEnabled(r) {
		fail(w, http.StatusConflict, "UPDATE_OFFLINE", "Offline mode is enabled; turn it off in Settings to check for updates.")
		return
	}
	body, err := fetchURLBytes("https://api.github.com/repos/" + updateRepository + "/releases/latest")
	if err != nil {
		fail(w, http.StatusBadGateway, "UPDATE_CHECK_FAILED", "Could not reach GitHub releases: "+err.Error())
		return
	}
	var release releaseInfo
	if err := json.Unmarshal(body, &release); err != nil || release.TagName == "" {
		fail(w, http.StatusBadGateway, "UPDATE_CHECK_FAILED", "GitHub returned an unexpected response.")
		return
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	notes := release.Body
	if len(notes) > 600 {
		notes = notes[:600] + "…"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current":       s.version,
		"latest":        latest,
		"available":     compareSemver(latest, s.version) > 0,
		"release_url":   release.HTMLURL,
		"release_notes": strings.TrimSpace(notes),
	})
}

// fetchInstallerScript downloads install-latest.ps1 pinned to the newest
// release tag, so the updater that runs always matches the release it is
// installing. main is the fallback for the short window before a tag exists
// or when the GitHub API call fails (rate limits and the like).
func fetchInstallerScript() ([]byte, error) {
	var release releaseInfo
	if body, err := fetchURLBytes("https://api.github.com/repos/" + updateRepository + "/releases/latest"); err == nil {
		_ = json.Unmarshal(body, &release)
	}
	if release.TagName != "" {
		if body, err := fetchURLBytes("https://raw.githubusercontent.com/" + updateRepository + "/" + release.TagName + "/scripts/install-latest.ps1"); err == nil {
			return body, nil
		}
	}
	return fetchURLBytes("https://raw.githubusercontent.com/" + updateRepository + "/main/scripts/install-latest.ps1")
}

// updateApply stages the official PowerShell installer plus a tiny launcher
// into a temp folder and starts it detached. The installer waits (see
// -WaitForCloseSeconds) for Blunt Code to exit; the UI triggers /system/stop
// right after this call returns, and once the installer finishes the launcher
// starts the freshly installed exe, so the handoff reads: click Update → app
// closes → installer swaps the binary → the new version opens on its own.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		fail(w, http.StatusNotImplemented, "UNSUPPORTED_PLATFORM", "In-app updates are Windows-only.")
		return
	}
	if s.offlineEnabled(r) {
		fail(w, http.StatusConflict, "UPDATE_OFFLINE", "Offline mode is enabled; turn it off in Settings to update.")
		return
	}
	raw, err := fetchInstallerScript()
	if err != nil {
		fail(w, http.StatusBadGateway, "UPDATE_APPLY_FAILED", "Could not download the installer: "+err.Error())
		return
	}
	dir, err := os.MkdirTemp("", "bluntcode-update-")
	if err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not stage the installer.")
		return
	}
	scriptPath := filepath.Join(dir, "install-latest.ps1")
	if err := os.WriteFile(scriptPath, raw, 0o600); err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not stage the installer.")
		return
	}
	launcherPath := filepath.Join(dir, "run-update.cmd")
	// The relaunch must target the installer's default location (not our own
	// os.Executable()): a portable run from Downloads reinstalls into the
	// default dir, and relaunching the old portable exe would undo the update.
	// The if-exist guard keeps the chain silent when the install failed or
	// landed somewhere custom.
	launcher := "@echo off\r\n" +
		"timeout /t 2 /nobreak >nul\r\n" +
		"powershell -NoProfile -ExecutionPolicy Bypass -File \"" + scriptPath + "\" -Silent -WaitForCloseSeconds 60\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo Update failed - the error above says why. Your current Blunt Code was kept;\r\n" +
		"  echo start it again from the Start menu.\r\n" +
		"  pause\r\n" +
		"  exit /b 1\r\n" +
		")\r\n" +
		"if exist \"%LOCALAPPDATA%\\Programs\\BluntCode\\bluntcode.exe\" start \"\" \"%LOCALAPPDATA%\\Programs\\BluntCode\\bluntcode.exe\"\r\n"
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o600); err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not stage the updater.")
		return
	}
	command := exec.Command("cmd", "/C", "start", "/min", "", launcherPath)
	command.Dir = dir
	if err := launchUpdateProcess(command); err != nil {
		fail(w, 500, "UPDATE_APPLY_FAILED", "Could not launch the installer.")
		return
	}
	s.log.Info("update staged", "dir", dir)
	writeJSON(w, http.StatusOK, map[string]any{"started": true, "staged_at": dir})
}

// launchUpdateProcess starts the detached updater and reaps it in the
// background. Package-level so tests can intercept without spawning anything.
var launchUpdateProcess = func(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return err
	}
	// Detached on purpose: the updater must outlive this HTTP request (and the
	// app shutdown that follows) to finish swapping the binary.
	go func() { _ = command.Wait() }()
	return nil
}