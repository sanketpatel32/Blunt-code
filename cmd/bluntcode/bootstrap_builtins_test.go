package main

import (
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/analyzers/dastprobe"
)

// TestOfflineKeepsLocalOnlyBuiltins pins issue #23: offline mode must keep
// the in-process analyzers that never touch the network, and keep the one
// outbound built-in (the DAST probe) confined to loopback.
func TestOfflineKeepsLocalOnlyBuiltins(t *testing.T) {
	for _, offline := range []bool{false, true} {
		registry := analyzers.NewRegistry()
		registerBuiltinAnalyzers(registry, offline)
		for _, id := range []string{"pentest", "secrets", "todo", "license-scan", dastprobe.ID} {
			adapter, ok := registry.Get(id)
			if !ok {
				t.Errorf("offline=%v: built-in %s not registered", offline, id)
				continue
			}
			if id != dastprobe.ID && !analyzers.AllowedOffline(id) {
				t.Errorf("%s is local-only and must be allowed offline", id)
			}
			if probe, isProbe := adapter.(*dastprobe.Adapter); isProbe && probe.Offline != offline {
				t.Errorf("dast-probe Offline = %v, want %v", probe.Offline, offline)
			}
		}
	}
}
