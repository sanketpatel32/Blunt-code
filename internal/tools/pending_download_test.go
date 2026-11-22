package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func testManifest() Manifest {
	return Manifest{Artifacts: []Artifact{
		{ToolID: "ruff", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/ruff.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "ruff.exe", LicenseURL: "https://example.invalid", DownloadBytes: 100},
		{ToolID: "uv", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/uv.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "uv.exe", LicenseURL: "https://example.invalid", DownloadBytes: 20},
		{ToolID: "semgrep", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/semgrep.whl", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "wheel", Executable: "env/semgrep/Scripts/python.exe", LicenseURL: "https://example.invalid", InstallKind: "uv_tool", Package: "semgrep==1.0.0", DownloadBytes: 50},
		{ToolID: "sonarqube-server", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/sonar.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "sonar/bin/start.bat", LicenseURL: "https://example.invalid", DownloadBytes: 900},
		{ToolID: "sonar-scanner", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/scanner.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "scanner/bin/scanner.bat", LicenseURL: "https://example.invalid", DownloadBytes: 40},
		{ToolID: "java", Version: "1.0.0", Platform: platform(), SourceURL: "https://example.invalid/java.zip", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArchiveType: "zip", Executable: "jdk/bin/java.exe", LicenseURL: "https://example.invalid", DownloadBytes: 200},
	}}
}

// markReady plants the executable IsReady stats, simulating an installed
// artifact without a network download.
func markReady(t *testing.T, m Manager, a Artifact) {
	t.Helper()
	path := m.Executable(a)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// PendingDownloadBytes is the size the scan's "Preparing X (~N MB)" stage
// quotes: a plain artifact reports itself, a uv_tool adds the uv bootstrap
// until it is installed, and SonarQube sums every missing bundle piece.
func TestPendingDownloadBytesSumsMissingPieces(t *testing.T) {
	service := NewService(t.TempDir(), testManifest(), false)
	if got := service.PendingDownloadBytes("ruff"); got != 100 {
		t.Fatalf("ruff pending = %d, want 100", got)
	}
	// semgrep needs its wheel plus the managed uv bootstrap it installs through.
	if got := service.PendingDownloadBytes("semgrep"); got != 70 {
		t.Fatalf("semgrep pending = %d, want 70 (wheel + uv)", got)
	}
	// sonarqube is a logical tool over three artifacts; all missing here.
	if got := service.PendingDownloadBytes("sonarqube"); got != 1140 {
		t.Fatalf("sonarqube pending = %d, want 1140", got)
	}
	if got := service.PendingDownloadBytes("nonexistent"); got != 0 {
		t.Fatalf("unknown tool pending = %d, want 0", got)
	}
}

func TestPendingDownloadBytesDropsInstalledPieces(t *testing.T) {
	manifest := testManifest()
	service := NewService(t.TempDir(), manifest, false)
	// Install the uv bootstrap so semgrep's estimate sheds it.
	uv, _ := manifest.Find("uv", platform())
	markReady(t, service.Manager, uv)
	if got := service.PendingDownloadBytes("semgrep"); got != 50 {
		t.Fatalf("semgrep pending after uv install = %d, want 50", got)
	}
	// Every sonarqube piece present → nothing left to download.
	for _, id := range []string{"sonarqube-server", "sonar-scanner", "java"} {
		artifact, _ := manifest.Find(id, platform())
		markReady(t, service.Manager, artifact)
	}
	if got := service.PendingDownloadBytes("sonarqube"); got != 0 {
		t.Fatalf("sonarqube pending after full install = %d, want 0", got)
	}
}
