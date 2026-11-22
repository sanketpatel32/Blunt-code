// Package selfupdate is the in-app updater. It never executes a remote
// script: the running binary downloads the release zip for an exact tag,
// verifies it (Ed25519 signature over the published SHA-256 line when a
// release key is compiled in, then the SHA-256 itself), extracts it, and
// hands the swap to a copy of the already-installed binary.
package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const Repository = "sanketpatel32/Blunt-code"

const (
	maxArchiveBytes   = 256 << 20
	maxSidecarBytes   = 4 << 10
	maxExtractedBytes = 1 << 30
	executableName    = "bluntcode.exe"
)

var tagPattern = regexp.MustCompile(`^v(\d+\.\d+\.\d+)$`)

// Fetch streams url into w, refusing bodies larger than limit. Package-level
// so tests can stub the network.
var Fetch = func(ctx context.Context, url string, w io.Writer, limit int64) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "BluntCode-Updater")
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned status %d", url, response.StatusCode)
	}
	written, err := io.Copy(w, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if written > limit {
		return fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return nil
}

// Assets names the files of one release, with download URLs pinned to the tag.
type Assets struct {
	Tag          string
	Version      string
	ArchiveName  string
	ArchiveURL   string
	ChecksumURL  string
	SignatureURL string
}

// AssetsFor derives the asset names and URLs from a release tag. URLs are
// built here rather than taken from the API response so nothing outside the
// pinned repository and tag can be substituted.
func AssetsFor(tag string) (Assets, error) {
	match := tagPattern.FindStringSubmatch(tag)
	if match == nil {
		return Assets{}, fmt.Errorf("release tag %q is not of the form vX.Y.Z", tag)
	}
	name := "BluntCode-" + match[1] + "-windows-amd64.zip"
	base := "https://github.com/" + Repository + "/releases/download/" + tag + "/"
	return Assets{
		Tag:          tag,
		Version:      match[1],
		ArchiveName:  name,
		ArchiveURL:   base + name,
		ChecksumURL:  base + name + ".sha256",
		SignatureURL: base + name + ".sha256.sig",
	}, nil
}

// VerifySignature checks an Ed25519 signature (base64, as written by
// cmd/releasesign) over the exact bytes of the .sha256 file.
func VerifySignature(publicKeyB64 string, checksumFile, signatureFile []byte) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKeyB64))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("compiled-in release signing key is malformed")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureFile)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("release signature is malformed")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), checksumFile, signature) {
		return errors.New("release signature does not match the release signing key")
	}
	return nil
}

// ParseChecksum reads a "<sha256>  <name>" line and requires the name to be
// the expected archive, so a signed checksum for one release cannot vouch for
// another release's zip.
func ParseChecksum(checksumFile []byte, archiveName string) ([]byte, error) {
	fields := strings.Fields(string(checksumFile))
	if len(fields) != 2 {
		return nil, errors.New("release checksum file is malformed")
	}
	sum, err := hex.DecodeString(strings.ToLower(fields[0]))
	if err != nil || len(sum) != sha256.Size {
		return nil, errors.New("release checksum is not a SHA-256 digest")
	}
	if strings.TrimPrefix(fields[1], "*") != archiveName {
		return nil, fmt.Errorf("release checksum names %q, expected %q", fields[1], archiveName)
	}
	return sum, nil
}

func fileSHA256(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}

// Extract unpacks archivePath into dest and returns the single top-level
// application folder, which must contain bluntcode.exe. Entries escaping
// dest, links, and oversized payloads are rejected.
func Extract(archivePath, dest string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open release archive: %w", err)
	}
	defer reader.Close()
	root, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	var total int64
	tops := map[string]bool{}
	for _, entry := range reader.File {
		name := strings.ReplaceAll(entry.Name, `\`, "/")
		clean := filepath.Clean(filepath.FromSlash(name))
		if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("release archive entry %q escapes the payload folder", entry.Name)
		}
		target := filepath.Join(root, clean)
		tops[strings.SplitN(filepath.ToSlash(clean), "/", 2)[0]] = true
		mode := entry.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		case !mode.IsRegular():
			return "", fmt.Errorf("release archive entry %q is not a regular file", entry.Name)
		}
		total += int64(entry.UncompressedSize64)
		if total > maxExtractedBytes {
			return "", errors.New("release archive expands beyond the size limit")
		}
		if err := extractFile(entry, target); err != nil {
			return "", err
		}
	}
	if len(tops) != 1 {
		return "", errors.New("release archive does not contain exactly one application folder")
	}
	var top string
	for name := range tops {
		top = name
	}
	payload := filepath.Join(root, top)
	if info, err := os.Stat(filepath.Join(payload, executableName)); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("release archive does not contain %s", executableName)
	}
	return payload, nil
}

func extractFile(entry *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(source, int64(entry.UncompressedSize64)+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if written != int64(entry.UncompressedSize64) {
		return fmt.Errorf("release archive entry %q has an inconsistent size", entry.Name)
	}
	return closeErr
}

// Staged is a downloaded, verified, and extracted release.
type Staged struct {
	Assets     Assets
	PayloadDir string
	Signed     bool
}

// Stage downloads the release for tag into dir, verifies it, and extracts it.
// When signingKey is non-empty the .sha256.sig asset is mandatory; there is
// no fallback to another ref or an unsigned path.
func Stage(ctx context.Context, tag, dir, signingKey string) (Staged, error) {
	assets, err := AssetsFor(tag)
	if err != nil {
		return Staged{}, err
	}
	checksum, err := fetchSmall(ctx, assets.ChecksumURL)
	if err != nil {
		return Staged{}, fmt.Errorf("download checksum: %w", err)
	}
	signed := strings.TrimSpace(signingKey) != ""
	if signed {
		// A pinned key makes the .sha256.sig asset mandatory — no silent
		// downgrade to checksum-only verification.
		signature, err := fetchSmall(ctx, assets.SignatureURL)
		if err != nil {
			return Staged{}, fmt.Errorf("download release signature: %w", err)
		}
		if err := VerifySignature(signingKey, checksum, signature); err != nil {
			return Staged{}, err
		}
	}
	expected, err := ParseChecksum(checksum, assets.ArchiveName)
	if err != nil {
		return Staged{}, err
	}
	archivePath := filepath.Join(dir, assets.ArchiveName)
	if err := fetchToFile(ctx, assets.ArchiveURL, archivePath); err != nil {
		return Staged{}, fmt.Errorf("download release archive: %w", err)
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return Staged{}, err
	}
	if hex.EncodeToString(actual) != hex.EncodeToString(expected) {
		return Staged{}, errors.New("release archive SHA-256 mismatch; nothing was installed")
	}
	payload, err := Extract(archivePath, filepath.Join(dir, "payload"))
	if err != nil {
		return Staged{}, err
	}
	_ = os.Remove(archivePath)
	return Staged{Assets: assets, PayloadDir: payload, Signed: signed}, nil
}

func fetchSmall(ctx context.Context, url string) ([]byte, error) {
	var buffer strings.Builder
	if err := Fetch(ctx, url, &buffer, maxSidecarBytes); err != nil {
		return nil, err
	}
	return []byte(buffer.String()), nil
}

func fetchToFile(ctx context.Context, url, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	fetchErr := Fetch(ctx, url, file, maxArchiveBytes)
	closeErr := file.Close()
	if fetchErr != nil {
		return fetchErr
	}
	return closeErr
}
