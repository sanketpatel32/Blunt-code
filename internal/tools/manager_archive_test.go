package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZIPInstallPreservesPinnedArchiveLayout(t *testing.T) {
	payload := archiveBytes(t, map[string]string{
		"sonarqube-26/bin/windows-x86-64/StartSonar.bat": "start",
		"sonarqube-26/lib/app.jar":                       "jar",
	})
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	artifact := Artifact{ToolID: "sonarqube-server", Version: "26", Platform: platform(), SourceURL: server.URL, SHA256: hex.EncodeToString(sum[:]), ArchiveType: "zip", Executable: "sonarqube-26/bin/windows-x86-64/StartSonar.bat"}
	manager := Manager{Root: t.TempDir()}
	if err := manager.InstallExecutable(context.Background(), artifact); err != nil {
		t.Fatal(err)
	}
	if !manager.IsReady(artifact) {
		t.Fatal("expected pinned nested executable after archive install")
	}
	if _, err := os.Stat(filepath.Join(manager.Root, artifact.ToolID, artifact.Version, "sonarqube-26", "lib", "app.jar")); err != nil {
		t.Fatalf("expected complete server archive extraction: %v", err)
	}
}

func TestZIPInstallRejectsTraversalBeforeWriting(t *testing.T) {
	payload := archiveBytes(t, map[string]string{"../outside.txt": "bad", "tool.exe": "tool"})
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	root := t.TempDir()
	artifact := Artifact{ToolID: "tool", Version: "1", Platform: platform(), SourceURL: server.URL, SHA256: hex.EncodeToString(sum[:]), ArchiveType: "zip", Executable: "tool.exe"}
	err := (Manager{Root: root}).InstallExecutable(context.Background(), artifact)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "outside.txt")); !os.IsNotExist(err) {
		t.Fatal("archive traversal wrote outside its private staging directory")
	}
}

func archiveBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
