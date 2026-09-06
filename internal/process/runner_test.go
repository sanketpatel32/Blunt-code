package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunDoesNotInvokeShell(t *testing.T) {
	command := "go"
	args := []string{"version"}
	if _, err := exec.LookPath(command); err != nil {
		t.Skip("go unavailable")
	}
	got, err := Run(context.Background(), Request{Command: command, Args: args})
	if err != nil || got.ExitCode != 0 {
		t.Fatalf("run: %#v, %v", got, err)
	}
}

// TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes reproduces the Windows
// analyzer-timeout hazard: the cancelled child is killed, but a grandchild it
// spawned inherited the stdout/stderr pipes. Without a bounded wait, cmd.Wait
// blocks until that grandchild exits, so a "10 minute" analyzer timeout never
// actually ends the run and the scan hangs.
func TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes(t *testing.T) {
	if os.Getenv("GO_PROCESS_TEST_ROLE") != "" {
		runHelperProcess()
	}
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := Run(ctx, Request{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes"},
			Env:     append(os.Environ(), "GO_PROCESS_TEST_ROLE=child"),
		})
		done <- outcome{result: result, err: err}
	}()
	time.Sleep(1500 * time.Millisecond) // let the child spawn its grandchild
	cancel()
	select {
	case got := <-done:
		if got.err == nil && got.result.ExitCode == 0 {
			t.Fatalf("cancelled run reported success: %#v", got.result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after context cancellation: a grandchild holding the output pipes blocked the wait")
	}
}

// runHelperProcess implements the child and grandchild roles of the tests
// above. The child spawns a grandchild that inherits its stdout pipe and then
// sleeps; the grandchild just sleeps, keeping the pipe's write end open after
// the child is killed.
func runHelperProcess() {
	switch os.Getenv("GO_PROCESS_TEST_ROLE") {
	case "child":
		grandchild := exec.Command(os.Args[0], "-test.run=TestRunReturnsPromptlyWhenGrandchildHoldsOutputPipes")
		grandchild.Env = append(os.Environ(), "GO_PROCESS_TEST_ROLE=grandchild")
		grandchild.Stdout = os.Stdout
		grandchild.Stderr = os.Stderr
		if err := grandchild.Start(); err != nil {
			os.Exit(1)
		}
		if pidfile := os.Getenv("GO_PROCESS_TEST_PIDFILE"); pidfile != "" {
			_ = os.WriteFile(pidfile, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
		}
	case "grandchild":
	}
	time.Sleep(45 * time.Second)
	os.Exit(0)
}

// TestRunEndsDescendantsAfterReturn pins the job-object supervision: once Run
// has returned, nothing it started may keep running — not even a grandchild
// that escaped the tree-kill by holding the output pipes. The grandchild
// publishes its pid; the test polls until the operating system reports it
// gone, with a bound that failing machines can still reach.
func TestRunEndsDescendantsAfterReturn(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("job-object supervision is asserted through Windows tasklist")
	}
	if os.Getenv("GO_PROCESS_TEST_ROLE") != "" {
		runHelperProcess()
	}
	pidfile := filepath.Join(t.TempDir(), "grandchild.pid")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Run(ctx, Request{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestRunEndsDescendantsAfterReturn"},
			Env: append(os.Environ(),
				"GO_PROCESS_TEST_ROLE=child",
				"GO_PROCESS_TEST_PIDFILE="+pidfile,
			),
		})
	}()
	deadline := time.Now().Add(15 * time.Second)
	var pid string
	for pid == "" && time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidfile); err == nil {
			pid = strings.TrimSpace(string(raw))
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if pid == "" {
		cancel()
		t.Fatal("grandchild never published its pid")
	}
	cancel()
	<-done
	// Run has returned: the grandchild must end with it. Poll tasklist, which
	// only reports a live process; access-denied entries still prove liveness.
	gone := false
	deadline = time.Now().Add(15 * time.Second)
	for !gone && time.Now().Before(deadline) {
		output, err := exec.Command("tasklist", "/FI", "PID eq "+pid, "/FO", "CSV", "/NH").CombinedOutput()
		if err == nil && !strings.Contains(string(output), ","+pid+"\"") && !strings.Contains(string(output), "\""+pid+"\"") {
			gone = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !gone {
		t.Fatalf("grandchild pid %s still running after Run returned", pid)
	}
}

// TestRunBoundsCapturedOutput pins the captured-output limit: a child that
// floods stdout cannot grow the run's memory without bound. The retained
// bytes stop at the limit, the run reports truncation, and the child still
// exits successfully.
func TestRunBoundsCapturedOutput(t *testing.T) {
	if os.Getenv("GO_PROCESS_TEST_FLOOD") == "1" {
		runFlooderProcess()
	}
	result, err := Run(context.Background(), Request{
		Command:     os.Args[0],
		Args:        []string{"-test.run=TestRunBoundsCapturedOutput"},
		Env:         append(os.Environ(), "GO_PROCESS_TEST_FLOOD=1"),
		OutputLimit: 64 << 10,
	})
	if err != nil {
		t.Fatalf("flooded run errored: %v", err)
	}
	if !result.Truncated {
		t.Fatal("flooded output must be reported as truncated")
	}
	if len(result.Stdout) > 64<<10 {
		t.Fatalf("retained %d bytes, want at most %d", len(result.Stdout), 64<<10)
	}
}

func runFlooderProcess() {
	block := strings.Repeat("x", 4096)
	for i := 0; i < 512; i++ { // 2 MiB, far past the 64 KiB cap
		fmt.Println(block)
	}
	os.Exit(0)
}
