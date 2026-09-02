// Package build pins the application version in one place. The CLI (--version,
// API /meta) and the scan pipeline (snapshot stamping and the incremental
// reuse identity) must agree; a second copy of the constant in internal/scans
// once drifted for several releases, mislabeling reports and letting stale
// incremental findings survive upgrades that should have invalidated them.
package build

const Version = "0.16.19"
