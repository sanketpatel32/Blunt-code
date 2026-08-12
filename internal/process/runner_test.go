package process

import (
	"context"
	"os/exec"
	"testing"
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
