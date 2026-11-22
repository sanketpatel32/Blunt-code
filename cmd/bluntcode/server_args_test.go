package main

import (
	"flag"
	"io"
	"testing"
)

func serverFlagSet() *flag.FlagSet {
	flags := flag.NewFlagSet("bluntcode", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Bool("no-browser", false, "")
	flags.Int("port", 8787, "")
	return flags
}

func positionalArgs(t *testing.T, args ...string) []string {
	t.Helper()
	positional, err := collectPositionals(serverFlagSet(), args)
	if err != nil {
		t.Fatalf("collectPositionals(%v): %v", args, err)
	}
	return positional
}

// The documented `bluntcode [path] [--no-browser] [--port N]` order put the
// path first, but flag.FlagSet stops at the first positional — flags after
// the path were silently treated as more positionals and rejected as "too
// many arguments". The collector keeps interleaving until input ends.
func TestCollectPositionalsAcceptsFlagsAfterPath(t *testing.T) {
	got := positionalArgs(t, "C:\\repo", "--no-browser", "--port", "8899")
	if len(got) != 1 || got[0] != "C:\\repo" {
		t.Fatalf("path-first order lost the path: %v", got)
	}
}

func TestCollectPositionalsAcceptsFlagsBeforePath(t *testing.T) {
	got := positionalArgs(t, "--no-browser", "C:\\repo")
	if len(got) != 1 || got[0] != "C:\\repo" {
		t.Fatalf("flags-first order lost the path: %v", got)
	}
}

func TestCollectPositionalsInterleaved(t *testing.T) {
	got := positionalArgs(t, "--port", "8899", "C:\\repo", "--no-browser")
	if len(got) != 1 || got[0] != "C:\\repo" {
		t.Fatalf("interleaved order lost the path: %v", got)
	}
}

func TestCollectPositionalsKeepsExtraPositionalsForRejection(t *testing.T) {
	got := positionalArgs(t, "a", "--no-browser", "b")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("extra positional args must survive for the caller's usage error: %v", got)
	}
}

func TestCollectPositionalsEmpty(t *testing.T) {
	if got := positionalArgs(t); len(got) != 0 {
		t.Fatalf("no args must yield no positionals: %v", got)
	}
	if got := positionalArgs(t, "--no-browser"); len(got) != 0 {
		t.Fatalf("flag-only args must yield no positionals: %v", got)
	}
}
