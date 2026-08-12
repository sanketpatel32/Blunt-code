package analyzers

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"bluntcode/internal/process"
)

// RunDirect executes one preplanned process without a shell. Arguments remain
// discrete all the way to os/exec, so paths cannot change command meaning.
func RunDirect(ctx context.Context, plan AnalyzerPlan, emit EventEmitter) (AnalyzerResult, error) {
	if len(plan.Commands) != 1 {
		return AnalyzerResult{Plan: plan}, fmt.Errorf("analyzer %s must provide exactly one command", plan.AnalyzerID)
	}
	c := plan.Commands[0]
	if c.Executable == "" {
		return AnalyzerResult{Plan: plan}, fmt.Errorf("analyzer %s executable is empty", plan.AnalyzerID)
	}
	if emit != nil {
		emit.Emit(ctx, "analyzer.command_started", map[string]any{"analyzer_id": plan.AnalyzerID})
	}
	started := time.Now()
	env := []string(nil)
	if len(c.Env) > 0 {
		env = mergedEnv(os.Environ(), c.Env)
	}
	run, err := process.Run(ctx, process.Request{Command: c.Executable, Args: c.Args, Dir: c.Dir, Env: env})
	result := AnalyzerResult{Plan: plan, Stdout: run.Stdout, Stderr: run.Stderr, ExitCode: run.ExitCode, StartedAt: started, FinishedAt: started.Add(run.Duration), OutputTruncated: run.Truncated}
	if err != nil {
		return result, err
	}
	if run.Truncated {
		return result, fmt.Errorf("analyzer %s output exceeded the %d MB safety limit", plan.AnalyzerID, process.DefaultOutputLimit>>20)
	}
	return result, nil
}

func mergedEnv(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		if value == "" {
			delete(values, key)
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
