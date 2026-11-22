package scans

import (
	"errors"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/dast"
)

// ErrTargetNeedsPentest rejects a target URL on a non-pentest scan: only the
// pentest profile performs dynamic probing, and silently ignoring the target
// would let a caller believe a static scan probed their app.
var ErrTargetNeedsPentest = errors.New("a target URL is only accepted with the pentest profile, the one profile that runs the DAST probe")

// NormalizeScanTarget validates a scan's DAST target for profile and returns
// the canonical URL the dast-probe analyzer will request. An empty target is
// valid for every profile (the scan stays static-only).
func NormalizeScanTarget(profile, target string) (string, error) {
	if target == "" {
		return "", nil
	}
	if profile != analyzers.ProfilePentest {
		return "", ErrTargetNeedsPentest
	}
	parsed, err := dast.NormalizeTarget(target)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func profileOrStandard(profile string) string {
	if profile == "" {
		return analyzers.ProfileStandard
	}
	return profile
}
