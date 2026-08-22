package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestEnsureInstallsVerifiedPinnedArtifact(t *testing.T) {
	payload := []byte("tool bytes")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	manifest := Manifest{Artifacts: []Artifact{{ToolID: "ruff", Version: "1.0.0", Platform: platform(), SourceURL: server.URL, SHA256: hex.EncodeToString(sum[:]), ArchiveType: "exe", Executable: "ruff.exe"}}}
	service := NewService(t.TempDir(), manifest, false)
	if err := service.Ensure(context.Background(), "ruff"); err != nil {
		t.Fatal(err)
	}
	if !service.Status("ruff").Ready {
		t.Fatalf("tool was not installed at %s", filepath.Join(service.Manager.Root, "ruff"))
	}
}

func TestEnsureSemgrepUsesPrivatePinnedUVAndExtractsRules(t *testing.T) {
	root := t.TempDir()
	uv := Artifact{ToolID: "uv", Version: "0.11.16", Platform: platform(), SourceURL: "https://example.test/uv.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "uv.exe"}
	wheelPayload := []byte("verified test Semgrep wheel")
	wheelHash := sha256.Sum256(wheelPayload)
	const wheelName = "semgrep-1.172.0-cp310-cp310-win_amd64.whl"
	semgrep := Artifact{ToolID: "semgrep", Version: "1.172.0", Platform: platform(), SourceURL: "https://example.test/" + wheelName, SHA256: hex.EncodeToString(wheelHash[:]), ArchiveType: "wheel", Executable: "semgrep.exe", InstallKind: "uv_tool", Package: "semgrep==1.172.0"}
	service := NewService(root, Manifest{Artifacts: []Artifact{uv, semgrep}}, false)
	if err := os.MkdirAll(filepath.Dir(service.Manager.Executable(uv)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(service.Manager.Executable(uv), []byte("managed uv"), 0o600); err != nil {
		t.Fatal(err)
	}
	semgrepPaths := service.Manager.SemgrepPaths(semgrep)
	if err := os.MkdirAll(filepath.Join(semgrepPaths.Root, "downloads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(semgrepPaths.Root, "downloads", wheelName), wheelPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	var gotExecutable, gotDir string
	var gotArgs, gotEnv []string
	service.Manager.RunCommand = func(_ context.Context, executable string, args []string, dir string, env []string) error {
		gotExecutable, gotArgs, gotDir, gotEnv = executable, append([]string(nil), args...), dir, append([]string(nil), env...)
		paths := service.Manager.SemgrepPaths(semgrep)
		return os.WriteFile(paths.Executable, []byte("managed semgrep"), 0o600)
	}
	if err := service.Ensure(context.Background(), "semgrep"); err != nil {
		t.Fatal(err)
	}
	paths := semgrepPaths
	if gotExecutable != service.Manager.Executable(uv) || gotDir != paths.Root {
		t.Fatalf("uv must run from managed paths: executable=%q dir=%q", gotExecutable, gotDir)
	}
	if want := []string{"tool", "install", "--managed-python", filepath.Join(paths.Root, "downloads", wheelName)}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("uv command = %#v, want %#v", gotArgs, want)
	}
	env := environmentMap(gotEnv)
	for key, want := range map[string]string{"UV_TOOL_DIR": paths.ToolDir, "UV_TOOL_BIN_DIR": paths.Root, "UV_CACHE_DIR": paths.CacheDir, "UV_PYTHON_INSTALL_DIR": paths.PythonDir, "UV_MANAGED_PYTHON": "1"} {
		if env[key] != want {
			t.Fatalf("%s = %q, want %q", key, env[key], want)
		}
	}
	if status := service.Status("semgrep"); !status.Ready || status.Version != "1.172.0" {
		t.Fatalf("unexpected Semgrep status: %#v", status)
	}
	if err := verifySemgrepRules(paths.RulesDir); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureSemgrepRestoresBundledRulesOffline(t *testing.T) {
	root := t.TempDir()
	semgrep := Artifact{ToolID: "semgrep", Version: "1.172.0", Platform: platform(), SourceURL: "https://example.test/semgrep.whl", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "wheel", Executable: "semgrep.exe", InstallKind: "uv_tool", Package: "semgrep==1.172.0"}
	service := NewService(root, Manifest{Artifacts: []Artifact{semgrep}}, true)
	paths := service.Manager.SemgrepPaths(semgrep)
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Executable, []byte("managed semgrep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Ensure(context.Background(), "semgrep"); err != nil {
		t.Fatalf("offline rules repair: %v", err)
	}
	if err := verifySemgrepRules(paths.RulesDir); err != nil {
		t.Fatal(err)
	}
}

func environmentMap(entries []string) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		for index, c := range entry {
			if c == '=' {
				out[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	return out
}

// TestConcurrentEnsureSerializesInstalls pins the shared install directory:
// two workspaces (or a scan and the tools page) may install the same missing
// tool at the same time. Installs must be serialized so concurrent callers
// cannot interleave downloads, staging renames, and readiness checks.
func TestConcurrentEnsureSerializesInstalls(t *testing.T) {
	root := t.TempDir()
	uv := Artifact{ToolID: "uv", Version: "0.11.16", Platform: platform(), SourceURL: "https://example.test/uv.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "uv.exe"}
	wheelPayload := []byte("verified test Semgrep wheel")
	wheelHash := sha256.Sum256(wheelPayload)
	const wheelName = "semgrep-1.172.0-cp310-cp310-win_amd64.whl"
	semgrep := Artifact{ToolID: "semgrep", Version: "1.172.0", Platform: platform(), SourceURL: "https://example.test/" + wheelName, SHA256: hex.EncodeToString(wheelHash[:]), ArchiveType: "wheel", Executable: "semgrep.exe", InstallKind: "uv_tool", Package: "semgrep==1.172.0"}
	service := NewService(root, Manifest{Artifacts: []Artifact{uv, semgrep}}, false)
	if err := os.MkdirAll(filepath.Dir(service.Manager.Executable(uv)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(service.Manager.Executable(uv), []byte("managed uv"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := service.Manager.SemgrepPaths(semgrep)
	if err := os.MkdirAll(filepath.Join(paths.Root, "downloads"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Root, "downloads", wheelName), wheelPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	active, peak := 0, 0
	service.Manager.RunCommand = func(context.Context, string, []string, string, []string) error {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(150 * time.Millisecond) // guarantee overlap when unsynchronized
		if err := os.WriteFile(paths.Executable, []byte("managed semgrep"), 0o600); err != nil {
			return err
		}
		return ExtractSemgrepRules(paths.RulesDir)
	}
	const callers = 4
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			errs[slot] = service.Ensure(context.Background(), "semgrep")
		}(i)
	}
	wg.Wait()
	for slot, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Ensure %d failed: %v", slot, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if peak > 1 {
		t.Fatalf("install commands overlapped (peak concurrency %d); Ensure must serialize installs", peak)
	}
}
