//go:build windows

package sonarqube

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// waitForProcess polls the live system snapshot until predicate holds.
func waitForProcess(t *testing.T, timeout time.Duration, predicate func([]processSnapshot) bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		procs, err := snapshotSystemProcesses()
		if err == nil && predicate(procs) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func TestSnapshotSystemProcessesSeesSpawnedProcess(t *testing.T) {
	ping, err := exec.LookPath("ping.exe")
	if err != nil {
		t.Skipf("ping.exe is unavailable: %v", err)
	}
	cmd := exec.Command(ping, "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if !waitForProcess(t, 5*time.Second, func(procs []processSnapshot) bool {
		for _, p := range procs {
			if p.PID == cmd.Process.Pid && strings.EqualFold(p.Image, ping) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("snapshot never reported pid %d with image %q", cmd.Process.Pid, ping)
	}
}

func TestSweepStrayServerProcessesEndsOrphan(t *testing.T) {
	ping, err := exec.LookPath("ping.exe")
	if err != nil {
		t.Skipf("ping.exe is unavailable: %v", err)
	}
	// `start /b` leaves ping running after cmd exits, orphaning it exactly
	// like the leaked managed JVMs: its parent no longer exists, so its
	// ancestor chain never reaches this test process.
	launcher := exec.Command("cmd.exe", "/c", "start", "", "/b", "ping", "-n", "30", "127.0.0.1")
	if err := launcher.Run(); err != nil {
		t.Fatalf("create orphaned process: %v", err)
	}
	if !waitForProcess(t, 10*time.Second, func(procs []processSnapshot) bool {
		return len(selectStrayServerProcesses(procs, ping, os.Getpid())) > 0
	}) {
		t.Fatal("orphaned ping never appeared in the snapshot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sweepStrayServerProcesses(ctx, ping, nil); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if waitForProcess(t, 10*time.Second, func(procs []processSnapshot) bool {
		return len(selectStrayServerProcesses(procs, ping, os.Getpid())) == 0
	}) {
		return
	}
	t.Fatal("sweep did not end the orphaned process")
}

func TestTrackProcessInKillOnCloseJobAssignsProcess(t *testing.T) {
	ping, err := exec.LookPath("ping.exe")
	if err != nil {
		t.Skipf("ping.exe is unavailable: %v", err)
	}
	cmd := exec.Command(ping, "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if err := trackProcessInKillOnCloseJob(cmd.Process); err != nil {
		t.Fatalf("track in kill-on-close job: %v", err)
	}
}

// TestKillOnCloseJobEndsTreeWhenHandleCloses proves the operating-system
// guarantee the fix relies on: closing the last job handle (which is what
// happens when the owning process dies, however abruptly) terminates every
// process in the job without any application code running.
func TestKillOnCloseJobEndsTreeWhenHandleCloses(t *testing.T) {
	ping, err := exec.LookPath("ping.exe")
	if err != nil {
		t.Skipf("ping.exe is unavailable: %v", err)
	}
	cmd := exec.Command(ping, "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// A dedicated job so the package's shared job is left untouched.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		t.Fatalf("configure job: %v", err)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatalf("open process: %v", err)
	}
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		t.Fatalf("assign to job: %v", err)
	}
	_ = windows.CloseHandle(handle)
	_ = windows.CloseHandle(job)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("closing the job handle did not end the tracked process")
	}
}
