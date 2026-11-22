package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseServerFlagsAcceptsPathBeforeFlags(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseServerFlags([]string{`C:\Projects\app`, "--no-browser", "--port", "8899"}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.path != `C:\Projects\app` || !cfg.noBrowser || cfg.port != 8899 {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseServerFlagsAcceptsFlagsBeforePath(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseServerFlags([]string{"--port", "0", "--no-browser", "."}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.path != "." || !cfg.noBrowser || cfg.port != 0 {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseServerFlagsRejectsExtraArgumentsWithActionableError(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseServerFlags([]string{"one", "two"}, &errOut)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errOut.String(), `unexpected arguments "two"`) || !strings.Contains(errOut.String(), "run 'bluntcode help' for usage") {
		t.Fatalf("error output = %q", errOut.String())
	}
}
