package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed llm.txt
var llmText string

// llms.txt is alias — embed same content via root file if present, else fallback to llmText
// We embed llm.txt only; llms.txt at repo root is byte-identical by convention.

const agentUsage = "usage: bluntcode agent [--help] | bluntcode agent docs | bluntcode agent scan <path> [scan flags]"

const llmUsage = "usage: bluntcode llm | bluntcode llms  (cat llm.txt to stdout)"

func printAgentHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code agent helper — for AI agents and automation")
	fmt.Fprintln(w)
	fmt.Fprintln(w, agentUsage)
	fmt.Fprintln(w, llmUsage)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  bluntcode agent              show this help")
	fmt.Fprintln(w, "  bluntcode agent docs         print llm.txt (agent guide) to stdout")
	fmt.Fprintln(w, "  bluntcode agent scan <path> [flags]  run scan with agent-friendly defaults (--json --quiet)")
	fmt.Fprintln(w, "  bluntcode llm                alias for 'agent docs'")
	fmt.Fprintln(w, "  bluntcode llms               alias for 'agent docs'")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Agent docs: llm.txt at repo root (also served as llms.txt), embedded in binary.")
	fmt.Fprintln(w, "For full scan flags see: bluntcode scan --help")
}

func runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if len(args) > 1 {
			fmt.Fprintf(stderr, "bluntcode agent: unknown args %q\n", strings.Join(args[1:], " "))
			fmt.Fprintln(stderr, agentUsage)
			return 2
		}
		printAgentHelp(stdout)
		return 0
	}
	switch args[0] {
	case "docs", "doc", "llm", "llms":
		if len(args) != 1 {
			fmt.Fprintf(stderr, "bluntcode agent docs: takes no extra args\n")
			fmt.Fprintln(stderr, agentUsage)
			return 2
		}
		_, _ = io.WriteString(stdout, llmText)
		if !strings.HasSuffix(llmText, "\n") {
			fmt.Fprintln(stdout)
		}
		return 0
	case "scan":
		// Agent-friendly scan: force --json --quiet if not already present, then delegate to scan
		// This keeps parsing stable: progress on stderr, JSON on stdout.
		forward := args[1:]
		hasJSON := false
		hasQuiet := false
		for _, a := range forward {
			if a == "--json" {
				hasJSON = true
			}
			if a == "--quiet" {
				hasQuiet = true
			}
		}
		if !hasJSON {
			forward = append(forward, "--json")
		}
		if !hasQuiet {
			forward = append(forward, "--quiet")
		}
		return runScanCommand(forward, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode agent: unknown subcommand %q\n", args[0])
		fmt.Fprintln(stderr, agentUsage)
		return 2
	}
}

func runLLM(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintf(stderr, "bluntcode llm: takes no args\n")
		fmt.Fprintln(stderr, llmUsage)
		return 2
	}
	// Also support `bluntcode llm --help`
	_ = stderr
	_, _ = io.WriteString(stdout, llmText)
	if !strings.HasSuffix(llmText, "\n") {
		fmt.Fprintln(stdout)
	}
	return 0
}

// ensure llm.txt is present at repo root for go:embed — copy if missing at build time
var _ = func() int {
	// Verify embedded content is non-empty at init time; if empty, warn on stderr when run
	if strings.TrimSpace(llmText) == "" {
		fmt.Fprintln(os.Stderr, "warning: embedded llm.txt is empty; run `cp llm.txt cmd/bluntcode/llm.txt`")
	}
	return 0
}()
