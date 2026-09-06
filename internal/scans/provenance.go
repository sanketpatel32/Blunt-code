package scans

// Scan provenance and drift detection (IMP-07). A scan's snapshot records
// not just what was selected but a digest of the bytes actually analyzed, the
// configuration that shaped the selection, and the schema versions of the
// discovery policy and fingerprint identity. At completion the input digest
// is recomputed: a mismatch means the workspace changed under the scan, and
// the result is relabeled (completed_with_warnings + drift flag) instead of
// standing as reproducible for bytes it no longer describes.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"sort"
	"strings"

	"bluntcode/internal/core"
)

// inputDigest folds the per-file content hashes into one digest over the
// whole analyzed input set. Sorting the path lines keeps the digest stable
// regardless of map iteration or discovery order; including each path means a
// rename changes the digest even when byte content is identical.
func inputDigest(hashes map[string]string) string {
	if len(hashes) == 0 {
		return ""
	}
	paths := make([]string, 0, len(hashes))
	for path := range hashes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	digest := sha256.New()
	for _, path := range paths {
		digest.Write([]byte(path))
		digest.Write([]byte{0})
		digest.Write([]byte(hashes[path]))
		digest.Write([]byte{'\n'})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// configDigest digests the workspace configuration that shaped the scan:
// ignore/exclude rules, discovery exclusions, and path overrides. Order is
// canonicalized so semantically identical configurations digest equally.
func configDigest(rules []core.WorkspaceRule, exclusions []string, overrides []core.PathOverride) string {
	sortedRules := append([]core.WorkspaceRule(nil), rules...)
	sort.Slice(sortedRules, func(i, j int) bool {
		if sortedRules[i].ID != sortedRules[j].ID {
			return sortedRules[i].ID < sortedRules[j].ID
		}
		return sortedRules[i].Pattern < sortedRules[j].Pattern
	})
	type ruleView struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Pattern string `json:"pattern"`
		Enabled bool   `json:"enabled"`
	}
	ruleViews := make([]ruleView, 0, len(sortedRules))
	for _, rule := range sortedRules {
		ruleViews = append(ruleViews, ruleView{ID: rule.ID, Type: rule.RuleType, Pattern: rule.Pattern, Enabled: rule.Enabled})
	}
	sortedExclusions := append([]string(nil), exclusions...)
	sort.Strings(sortedExclusions)
	sortedOverrides := append([]core.PathOverride(nil), overrides...)
	sort.Slice(sortedOverrides, func(i, j int) bool {
		if sortedOverrides[i].RelativePath != sortedOverrides[j].RelativePath {
			return sortedOverrides[i].RelativePath < sortedOverrides[j].RelativePath
		}
		return sortedOverrides[i].Mode < sortedOverrides[j].Mode
	})
	payload, err := json.Marshal(struct {
		Rules      []ruleView          `json:"rules"`
		Exclusions []string            `json:"exclusions"`
		Overrides  []core.PathOverride `json:"overrides"`
	}{Rules: ruleViews, Exclusions: sortedExclusions, Overrides: sortedOverrides})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// dependencyDigest digests the dependency manifests and lockfiles the scan
// consumed, so vulnerability results can be tied to the exact input set even
// though lockfiles sit outside SelectedFiles.
func dependencyDigest(inputs []string) string {
	if len(inputs) == 0 {
		return ""
	}
	sorted := append([]string(nil), inputs...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

// platformSnapshot records the execution environment alongside the analyzer
// versions: the same workspace on a different OS or architecture can produce
// different analyzer behavior.
func platformSnapshot() map[string]string {
	return map[string]string{
		"os":   runtime.GOOS,
		"arch": runtime.GOARCH,
		"go":   runtime.Version(),
	}
}
