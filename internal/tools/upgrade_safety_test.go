package tools

// IMP-10 acceptance: hash mismatch, malformed archives, interruption, and a
// failed smoke test can never leave an unverified executable active, a failed
// update never destroys the previously working version, and offline mode
// fails readiness explicitly without network attempts.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serveBytes(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	t.Cleanup(server.Close)
	return server
}

func exeArtifact(serverURL, sum string) Artifact {
	return Artifact{ToolID: "probe", Version: "1.0.0", Platform: platform(), SourceURL: serverURL, SHA256: sum, ArchiveType: "exe", Executable: "probe.exe"}
}

func sha256Hex(t *testing.T, payload []byte) string {
	t.Helper()
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// A same-version reinstall whose smoke probe fails must leave the previously
// working binary in place and no backup litter behind.
func TestSmokeFailureKeepsPreviousExecutable(t *testing.T) {
	oldBytes := []byte("previous-working-binary")
	server := serveBytes(t, oldBytes)
	artifact := exeArtifact(server.URL, sha256Hex(t, oldBytes))
	root := t.TempDir()
	installed := Manager{Root: root, SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "probe 1.0.0", nil
	}}
	artifact.Smoke = &SmokeTest{Args: []string{"--version"}, Expect: "1.0.0"}
	if err := installed.InstallExecutable(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}

	failing := Manager{Root: root, SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "", errors.New("exit status 1")
	}}
	if err := failing.InstallExecutable(context.Background(), artifact); err == nil || !strings.Contains(err.Error(), "smoke test") {
		t.Fatalf("want smoke-test failure, got %v", err)
	}
	got, err := os.ReadFile(installed.Executable(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(oldBytes) {
		t.Fatal("failed update destroyed or replaced the previously working binary")
	}
	if _, err := os.Stat(installed.Executable(artifact) + ".previous"); !os.IsNotExist(err) {
		t.Fatal("rollback litter (.previous) left behind")
	}
}

// A probe that runs but prints the wrong version is just as rejected as one
// that fails to run.
func TestSmokeVersionMismatchRejected(t *testing.T) {
	payload := []byte("binary")
	server := serveBytes(t, payload)
	artifact := exeArtifact(server.URL, sha256Hex(t, payload))
	artifact.Smoke = &SmokeTest{Args: []string{"--version"}, Expect: "1.0.0"}
	manager := Manager{Root: t.TempDir(), SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "probe 9.9.9", nil
	}}
	if err := manager.InstallExecutable(context.Background(), artifact); err == nil || !strings.Contains(err.Error(), "did not mention") {
		t.Fatalf("want version-mismatch rejection, got %v", err)
	}
	if manager.IsReady(artifact) {
		t.Fatal("an unverified executable was left active")
	}
}

// A zip reinstall over a broken installation (executable deleted, tree
// remains) whose smoke probe fails restores the pre-reinstall tree exactly —
// it can never make things worse, but it cannot resurrect the deleted file
// either; a later clean reinstall with a passing probe repairs it.
func TestZIPSmokeFailureRestoresPreviousTree(t *testing.T) {
	payload := archiveBytes(t, map[string]string{"tool.exe": "old", "data/lib.txt": "lib"})
	server := serveBytes(t, payload)
	artifact := Artifact{ToolID: "tool", Version: "1", Platform: platform(), SourceURL: server.URL, SHA256: sha256Hex(t, payload), ArchiveType: "zip", Executable: "tool.exe"}
	root := t.TempDir()
	manager := Manager{Root: root}
	if err := manager.InstallExecutable(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(root, "tool", "1")
	// Corrupt the installation: executable gone, tree remains → not ready.
	if err := os.Remove(filepath.Join(destDir, "tool.exe")); err != nil {
		t.Fatal(err)
	}
	artifact.Smoke = &SmokeTest{Args: []string{"--version"}}
	failing := Manager{Root: root, SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "", errors.New("probe crashed")
	}}
	if err := failing.InstallExecutable(context.Background(), artifact); err == nil {
		t.Fatal("smoke failure must fail the installation")
	}
	// The pre-reinstall tree is intact and no staging or backup litter is left.
	if _, err := os.Stat(filepath.Join(destDir, "data", "lib.txt")); err != nil {
		t.Fatalf("previous tree was not restored after failed smoke test: %v", err)
	}
	if manager.IsReady(artifact) {
		t.Fatal("a tree missing its executable must not read as ready")
	}
	if _, err := os.Stat(destDir + ".previous"); !os.IsNotExist(err) {
		t.Fatal("rollback litter (.previous) left behind")
	}
	// A clean reinstall with a passing probe repairs the installation.
	repair := Manager{Root: root, SmokeRunner: func(context.Context, string, []string) (string, error) {
		return "tool 1", nil
	}}
	if err := repair.InstallExecutable(context.Background(), artifact); err != nil {
		t.Fatalf("repair reinstall: %v", err)
	}
	if !repair.IsReady(artifact) {
		t.Fatal("reinstall with passing smoke test did not repair the installation")
	}
}

// Every installation records its provenance beside the binary: what was
// installed, from where, and how it was verified.
func TestInstallMetadataRecordsProvenance(t *testing.T) {
	payload := archiveBytes(t, map[string]string{"tool.exe": "bin"})
	server := serveBytes(t, payload)
	artifact := Artifact{ToolID: "tool", Version: "1", Platform: platform(), SourceURL: server.URL, SHA256: sha256Hex(t, payload), ArchiveType: "zip", Executable: "tool.exe"}
	root := t.TempDir()
	if err := (Manager{Root: root}).InstallExecutable(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "tool", "1", installMetadataName))
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tool_id", "version", "source_url", "sha256", "verification", "installed_at_utc", "bluntcode_version"} {
		if record[key] == nil || record[key] == "" {
			t.Fatalf("install metadata lacks %s: %s", key, raw)
		}
	}
	if record["sha256"] != artifact.SHA256 {
		t.Fatalf("metadata sha256 = %v, want the pinned digest", record["sha256"])
	}
}

// SweepStaging recovers the exact on-disk states an interrupted update can
// leave: stale download temps, stale extraction staging, a completed
// activation whose cleanup died, and an activation that died between the two
// renames.
func TestSweepStagingRecoversInterruptedUpdates(t *testing.T) {
	root := t.TempDir()
	mkdir := func(rel string) string {
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Stale download temp.
	downloads := mkdir(".downloads")
	write(".downloads/tool-123.partial", "half")
	// Stale extraction staging.
	mkdir("tool/1.0.0.new-123")
	// Interrupted activation: backup exists, destination missing.
	mkdir("tool/2.0.0.previous")
	write("tool/2.0.0.previous/tool.exe", "previous")
	// Completed activation whose cleanup died: destination present AND backup.
	mkdir("tool/3.0.0")
	write("tool/3.0.0/tool.exe", "current")
	mkdir("tool/3.0.0.previous")
	// A normal, untouched installation.
	mkdir("tool/4.0.0")
	write("tool/4.0.0/tool.exe", "untouched")

	Manager{Root: root}.SweepStaging()

	if _, err := os.Stat(filepath.Join(downloads, "tool-123.partial")); !os.IsNotExist(err) {
		t.Fatal("stale download temp survived the sweep")
	}
	if _, err := os.Stat(filepath.Join(root, "tool", "1.0.0.new-123")); !os.IsNotExist(err) {
		t.Fatal("stale staging directory survived the sweep")
	}
	restored, err := os.ReadFile(filepath.Join(root, "tool", "2.0.0", "tool.exe"))
	if err != nil || string(restored) != "previous" {
		t.Fatalf("interrupted activation was not restored: %v %q", err, restored)
	}
	if _, err := os.Stat(filepath.Join(root, "tool", "2.0.0.previous")); !os.IsNotExist(err) {
		t.Fatal("consumed backup survived the sweep")
	}
	if _, err := os.Stat(filepath.Join(root, "tool", "3.0.0.previous")); !os.IsNotExist(err) {
		t.Fatal("stale backup of a completed activation survived the sweep")
	}
	if got, err := os.ReadFile(filepath.Join(root, "tool", "3.0.0", "tool.exe")); err != nil || string(got) != "current" {
		t.Fatal("sweep must not touch a completed activation")
	}
	if got, err := os.ReadFile(filepath.Join(root, "tool", "4.0.0", "tool.exe")); err != nil || string(got) != "untouched" {
		t.Fatal("sweep must not touch untouched installations")
	}
}

// Offline mode with a missing asset is an explicit readiness failure with no
// network attempt: the artifact's source URL points at a server that fails
// the test the moment it is contacted.
func TestOfflineMissingToolFailsWithoutNetwork(t *testing.T) {
	var contacted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()
	manifest := Manifest{Artifacts: []Artifact{{ToolID: "probe", Version: "1", Platform: platform(), SourceURL: server.URL, SHA256: sha256Hex(t, []byte("x")), ArchiveType: "exe", Executable: "probe.exe"}}}
	service := NewService(t.TempDir(), manifest, true)
	if err := service.Ensure(context.Background(), "probe"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("want explicit offline failure, got %v", err)
	}
	if contacted {
		t.Fatal("offline mode attempted a network download")
	}
}
