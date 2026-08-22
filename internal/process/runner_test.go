package process

import (
	"context"
	"os"
	"os/exec"
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

// runHelperProcess implements the child and grandchild roles of the test above.
// The child spawns a grandchild that inherits its stdout pipe and then sleeps;
// the grandchild just sleeps, keeping the pipe's write end open after the
// child is killed.
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
	case "grandchild":
	}
	time.Sleep(45 * time.Second)
	os.Exit(0)
}
