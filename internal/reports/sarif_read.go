package reports

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// The reading half of the SARIF export. The writer (sarif.go) is the only
// producer this project controls, but `bluntcode scan --baseline <file>` must
// accept any file the writer ever emitted, so the reader decodes leniently
// (unknown properties are ignored) and validates only what it consumes: the
// SARIF version and, per result, the fingerprint property bags.

// SARIFFinding is one fingerprinted result of a parsed SARIF log. Level is the
// result's severity level (error, warning, note) when it carries one; the
// three SARIF levels cannot reproduce the five Blunt Code severities, so the
// gate always reads severity from the current scan's findings and uses the
// baseline purely as a fingerprint set.
type SARIFFinding struct {
	Fingerprint string
	Level       string
}

// reader-side log shapes. They deliberately mirror only the fields ReadSARIF
// consumes so writer-side changes to rules, messages, or locations never break
// baseline loading.
type sarifReadLog struct {
	Version string         `json:"version"`
	Runs    []sarifReadRun `json:"runs"`
}
type sarifReadRun struct {
	Results []sarifReadResult `json:"results"`
}
type sarifReadResult struct {
	Level               string            `json:"level"`
	Fingerprints        map[string]string `json:"fingerprints"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

// ReadSARIF parses a SARIF 2.1.0 log and returns one SARIFFinding per result
// that carries a fingerprint. A result's fingerprint prefers the
// SARIFFingerprintKey entry (Blunt Code's own) in either property bag; without
// that key the stable fingerprints bag wins over partialFingerprints, taking
// the first non-empty value in sorted key order, so third-party logs stay
// usable as baselines. Results without any fingerprint are skipped, not an
// error: they simply cannot be matched. Anything else malformed — undecodable
// JSON, a missing or unsupported version — is an error so callers can reject
// the baseline before scanning.
func ReadSARIF(r io.Reader) ([]SARIFFinding, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read SARIF: %w", err)
	}
	var log sarifReadLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("invalid SARIF: %w", err)
	}
	if log.Version != sarifVersion {
		return nil, fmt.Errorf("invalid SARIF: version %q is not %s", log.Version, sarifVersion)
	}
	findings := make([]SARIFFinding, 0)
	for _, run := range log.Runs {
		for _, result := range run.Results {
			fingerprint := sarifResultFingerprint(result)
			if fingerprint == "" {
				continue
			}
			findings = append(findings, SARIFFinding{Fingerprint: fingerprint, Level: strings.TrimSpace(result.Level)})
		}
	}
	return findings, nil
}

// sarifResultFingerprint resolves one result's fingerprint deterministically:
// the Blunt Code key wins in either property bag; otherwise the stable
// fingerprints bag is preferred over partialFingerprints (SARIF's own rule for
// result identity), and the fallback picks the first non-empty value in sorted
// key order (map iteration order is random in Go, so "first" must be explicit).
func sarifResultFingerprint(result sarifReadResult) string {
	bags := []map[string]string{result.PartialFingerprints, result.Fingerprints}
	for _, bag := range bags {
		if value := strings.TrimSpace(bag[SARIFFingerprintKey]); value != "" {
			return value
		}
	}
	for _, bag := range []map[string]string{result.Fingerprints, result.PartialFingerprints} {
		keys := make([]string, 0, len(bag))
		for key := range bag {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if value := strings.TrimSpace(bag[key]); value != "" {
				return value
			}
		}
	}
	return ""
}
