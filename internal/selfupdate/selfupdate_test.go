package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type fakeRelease struct {
	assets map[string][]byte
	seen   []string
}

func (f *fakeRelease) install(t *testing.T) {
	t.Helper()
	original := Fetch
	Fetch = func(_ context.Context, url string, w io.Writer, limit int64) error {
		f.seen = append(f.seen, url)
		body, ok := f.assets[url]
		if !ok {
			return fmt.Errorf("404 %s", url)
		}
		if int64(len(body)) > limit {
			return fmt.Errorf("too large")
		}
		_, err := w.Write(body)
		return err
	}
	t.Cleanup(func() { Fetch = original })
}

func newRelease(t *testing.T, tag string, archive []byte, private ed25519.PrivateKey) *fakeRelease {
	t.Helper()
	assets, err := AssetsFor(tag)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	checksum := []byte(hex.EncodeToString(sum[:]) + "  " + assets.ArchiveName)
	release := &fakeRelease{assets: map[string][]byte{
		assets.ArchiveURL:  archive,
		assets.ChecksumURL: checksum,
	}}
	if private != nil {
		release.assets[assets.SignatureURL] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, checksum)))
	}
	return release
}

func validArchive(t *testing.T) []byte {
	return buildZip(t, map[string]string{
		"BluntCode-1.2.3-windows-amd64/bluntcode.exe": "new binary",
		"BluntCode-1.2.3-windows-amd64/README.md":     "readme",
	})
}

func TestAssetsForPinsURLsToTag(t *testing.T) {
	assets, err := AssetsFor("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/sanketpatel32/Blunt-code/releases/download/v1.2.3/BluntCode-1.2.3-windows-amd64.zip"
	if assets.ArchiveURL != want || assets.ChecksumURL != want+".sha256" || assets.SignatureURL != want+".sha256.sig" {
		t.Fatalf("unexpected assets: %+v", assets)
	}
	for _, bad := range []string{"main", "v1.2", "v1.2.3/../../main", "1.2.3", ""} {
		if _, err := AssetsFor(bad); err == nil {
			t.Errorf("AssetsFor(%q) accepted a non-release ref", bad)
		}
	}
}

func TestStageVerifiesSignedRelease(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	release := newRelease(t, "v1.2.3", validArchive(t), private)
	release.install(t)
	staged, err := Stage(context.Background(), "v1.2.3", t.TempDir(), base64.StdEncoding.EncodeToString(public))
	if err != nil {
		t.Fatal(err)
	}
	if !staged.Signed {
		t.Fatal("staged release not marked signed")
	}
	body, err := os.ReadFile(filepath.Join(staged.PayloadDir, "bluntcode.exe"))
	if err != nil || string(body) != "new binary" {
		t.Fatalf("payload not extracted: %v %q", err, body)
	}
	for _, url := range release.seen {
		if !strings.Contains(url, "/releases/download/v1.2.3/") {
			t.Fatalf("fetched outside the pinned tag: %s", url)
		}
	}
}

func TestStageRejectsBadSignatureAndMissingSignature(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	_, attacker, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(public)

	newRelease(t, "v1.2.3", validArchive(t), attacker).install(t)
	if _, err := Stage(context.Background(), "v1.2.3", t.TempDir(), key); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("release signed by another key was accepted: %v", err)
	}

	newRelease(t, "v1.2.3", validArchive(t), nil).install(t)
	if _, err := Stage(context.Background(), "v1.2.3", t.TempDir(), key); err == nil {
		t.Fatal("unsigned release was accepted while a signing key is compiled in")
	}
}

func TestStageRejectsChecksumMismatch(t *testing.T) {
	release := newRelease(t, "v1.2.3", validArchive(t), nil)
	assets, _ := AssetsFor("v1.2.3")
	release.assets[assets.ArchiveURL] = buildZip(t, map[string]string{"BluntCode-1.2.3-windows-amd64/bluntcode.exe": "tampered"})
	release.install(t)
	dir := t.TempDir()
	if _, err := Stage(context.Background(), "v1.2.3", dir, ""); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tampered archive was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "payload")); !os.IsNotExist(err) {
		t.Fatal("tampered archive was extracted")
	}
}

func TestParseChecksumBindsArchiveName(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	if _, err := ParseChecksum([]byte(sum+"  BluntCode-1.2.3-windows-amd64.zip"), "BluntCode-1.2.3-windows-amd64.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseChecksum([]byte(sum+"  BluntCode-1.0.0-windows-amd64.zip"), "BluntCode-1.2.3-windows-amd64.zip"); err == nil {
		t.Fatal("checksum for another release was accepted")
	}
	if _, err := ParseChecksum([]byte("zz  BluntCode-1.2.3-windows-amd64.zip"), "BluntCode-1.2.3-windows-amd64.zip"); err == nil {
		t.Fatal("malformed digest was accepted")
	}
}

func TestExtractRejectsEscapesAndMultipleRoots(t *testing.T) {
	cases := map[string]map[string]string{
		"escape":   {"../evil.exe": "x", "app/bluntcode.exe": "x"},
		"two-root": {"a/bluntcode.exe": "x", "b/bluntcode.exe": "x"},
		"no-exe":   {"app/README.md": "x"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a.zip")
			if err := os.WriteFile(archive, buildZip(t, files), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Extract(archive, filepath.Join(dir, "out")); err == nil {
				t.Fatal("unsafe archive was accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "evil.exe")); err == nil {
				t.Fatal("entry escaped the payload folder")
			}
		})
	}
}

func TestInstallSwapsAndRollsBack(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Programs", "BluntCode")
	if err := os.MkdirAll(install, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "bluntcode.exe"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(root, "missing")
	if err := Install(missing, install); err == nil {
		t.Fatal("install from a missing payload succeeded")
	}
	if body, _ := os.ReadFile(filepath.Join(install, "bluntcode.exe")); string(body) != "old" {
		t.Fatalf("previous install not kept: %q", body)
	}

	payload := filepath.Join(root, "payload", "app")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "bluntcode.exe"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(payload, install); err != nil {
		t.Fatal(err)
	}
	if body, _ := os.ReadFile(filepath.Join(install, "bluntcode.exe")); string(body) != "new" {
		t.Fatalf("new version not installed: %q", body)
	}
	entries, _ := os.ReadDir(filepath.Dir(install))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".bluntcode-backup-") {
			t.Fatalf("backup left behind: %s", entry.Name())
		}
	}
}

func TestVerifySignatureRejectsMalformedKey(t *testing.T) {
	if err := VerifySignature("not-base64!", []byte("x"), []byte("y")); err == nil {
		t.Fatal("malformed key accepted")
	}
}
