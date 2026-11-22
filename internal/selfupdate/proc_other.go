//go:build !windows

package selfupdate

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func waitForExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delay := 50 * time.Millisecond
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process %d did not exit within %s", pid, timeout)
		}
		time.Sleep(delay)
		if delay < 800*time.Millisecond {
			delay *= 2
		}
	}
}

// StartDetached starts command in its own session so it outlives the caller.
func StartDetached(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
