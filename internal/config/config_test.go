package config

import (
	"testing"
)

func TestMaxScansPerWorkspace(t *testing.T) {
	// Default when unset
	t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", "")
	if got := MaxScansPerWorkspace(); got != DefaultMaxScansPerWorkspace {
		t.Fatalf("expected default %d, got %d", DefaultMaxScansPerWorkspace, got)
	}

	// Valid positive integers
	t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", "5")
	if got := MaxScansPerWorkspace(); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}

	t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", "500")
	if got := MaxScansPerWorkspace(); got != 500 {
		t.Fatalf("expected 500, got %d", got)
	}

	// Disabling retention via 0 / off / none / disable / disabled
	for _, val := range []string{"0", "off", "OFF", "none", "disable", "disabled", "  0  "} {
		t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", val)
		if got := MaxScansPerWorkspace(); got != 0 {
			t.Fatalf("expected 0 (disabled) for %q, got %d", val, got)
		}
	}

	// Invalid strings fall back to default
	for _, val := range []string{"invalid", "-5", "abc"} {
		t.Setenv("BLUNTCODE_MAX_SCANS_PER_WORKSPACE", val)
		if got := MaxScansPerWorkspace(); got != DefaultMaxScansPerWorkspace {
			t.Fatalf("expected fallback default %d for %q, got %d", DefaultMaxScansPerWorkspace, val, got)
		}
	}
}
