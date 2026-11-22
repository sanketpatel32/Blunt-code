package analyzers

import "path/filepath"

// RelativePath returns path relative to root in forward-slash form. It falls
// back to path itself when root is empty or the two cannot be related.
func RelativePath(root, path string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// PlanRelativePath resolves path against the working directory of the plan's
// first command, the directory the analyzer reports its paths from.
func PlanRelativePath(p AnalyzerPlan, path string) string {
	if len(p.Commands) == 0 {
		return filepath.ToSlash(path)
	}
	return RelativePath(p.Commands[0].Dir, path)
}
