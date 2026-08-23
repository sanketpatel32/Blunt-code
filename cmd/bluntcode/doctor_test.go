package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseDoctorFlagsDefaultsToDiagnosticsOnly(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseDoctorFlags(nil, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.json || cfg.fix {
		t.Fatalf("cfg = %#v", cfg)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected output: %q", errOut.String())
	}
}

func TestParseDoctorFlagsAcceptsFixAloneAndWithJSON(t *testing.T) {
	cases := []struct {
		args []string
		json bool
	}{
		{args: []string{"--fix"}, json: false},
		{args: []string{"--fix", "--json"}, json: true},
		{args: []string{"--json", "--fix"}, json: true},
	}
	for _, item := range cases {
		var errOut bytes.Buffer
		cfg, err := parseDoctorFlags(item.args, &errOut)
		if err != nil {
			t.Fatalf("parse %v: %v", item.args, err)
		}
		if !cfg.fix || cfg.json != item.json {
			t.Fatalf("parse %v = %#v, want fix=true json=%v", item.args, cfg, item.json)
		}
		if errOut.Len() != 0 {
			t.Fatalf("parse %v printed %q", item.args, errOut.String())
		}
	}
}

func TestParseDoctorFlagsFixTakesNoValue(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := parseDoctorFlags([]string{"--fix", "true"}, &errOut); err == nil {
		t.Fatal("--fix must not consume a value; a trailing argument is a usage error")
	}
	if !strings.Contains(errOut.String(), doctorUsage) {
		t.Fatalf("usage not printed: %q", errOut.String())
	}
	var valueErrOut bytes.Buffer
	if _, err := parseDoctorFlags([]string{"--fix=maybe"}, &valueErrOut); err == nil {
		t.Fatal("--fix accepts only boolean values")
	}
}

func TestParseDoctorFlagsRejectsUnknownInput(t *testing.T) {
	var flagErrOut bytes.Buffer
	if _, err := parseDoctorFlags([]string{"--bogus"}, &flagErrOut); err == nil {
		t.Fatal("unknown flag must error")
	}
	var argErrOut bytes.Buffer
	if _, err := parseDoctorFlags([]string{"--json", "extra"}, &argErrOut); err == nil {
		t.Fatal("positional argument must error")
	}
	if !strings.Contains(argErrOut.String(), doctorUsage) {
		t.Fatalf("usage not printed: %q", argErrOut.String())
	}
}
