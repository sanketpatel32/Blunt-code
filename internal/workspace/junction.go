package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// junctionHopLimit bounds how many reparse points (junctions or symlinks) one
// path component may resolve through, so cyclic links cannot loop forever.
const junctionHopLimit = 16

// ResolveJunctions canonicalizes an absolute path by resolving directory
// junctions that filepath.EvalSymlinks skips: on Windows a junction is not
// reported as a symlink, yet os.Readlink still returns its target. Every
// component is checked, so a junction anywhere in the path is resolved.
// Resolution is best effort: any component that cannot be resolved is kept
// as-is, and if the fully resolved path is no longer an existing directory
// the original path is returned unchanged.
func ResolveJunctions(abs string) string {
	if abs == "" {
		return abs
	}
	volume := filepath.VolumeName(abs)
	separator := string(filepath.Separator)
	rest := strings.TrimPrefix(filepath.Clean(abs), volume+separator)
	current := volume + separator
	for _, part := range strings.Split(rest, separator) {
		if part == "" {
			continue
		}
		next := current + part
		for hop := 0; hop < junctionHopLimit; hop++ {
			target, err := os.Readlink(next)
			if err != nil {
				break
			}
			// Junction targets are reported with an NT object-manager prefix.
			target = strings.TrimPrefix(target, `\??\`)
			if !filepath.IsAbs(target) {
				target = filepath.Join(current, target)
			}
			next = filepath.Clean(target)
		}
		current = next + separator
	}
	resolved := filepath.Clean(current)
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return abs
	}
	return resolved
}
