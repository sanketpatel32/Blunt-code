//go:build windows

package sonarqube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sweepStrayServerProcesses ends every process running the managed server's
// pinned Java executable that is not part of this process's own live tree.
// It heals the leak behind the 2026-09-02 "managed SonarQube stopped during
// startup" failure: a previous session died without reaping its JVMs, the
// orphans kept locks over the shared runtime, and every later boot failed at
// the plugin deploy cleanup. Executables that are not absolute paths are
// never matched, because the system image paths being compared are absolute.
func sweepStrayServerProcesses(ctx context.Context, executable string, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	if executable == "" || !filepath.IsAbs(executable) {
		return nil
	}
	procs, err := snapshotSystemProcesses()
	if err != nil {
		return err
	}
	strays := selectStrayServerProcesses(procs, executable, os.Getpid())
	for _, pid := range strays {
		logger.Warn("ending stray managed SonarQube process from a previous session", "pid", pid)
		if err := killWindowsProcessTree(ctx, pid); err != nil {
			logger.Error("failed to end stray managed SonarQube process", "pid", pid, "error", err)
			return err
		}
	}
	return nil
}

// snapshotSystemProcesses lists every running process with its parent id and
// full executable image path. Protected system processes whose image cannot
// be opened resolve to an empty path and simply never match the managed
// executable.
func snapshotSystemProcesses() ([]processSnapshot, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	var out []processSnapshot
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("read first process entry: %w", err)
	}
	for {
		out = append(out, processSnapshot{
			PID:   int(entry.ProcessID),
			PPID:  int(entry.ParentProcessID),
			Image: processImagePath(entry.ProcessID),
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return nil, fmt.Errorf("read next process entry: %w", err)
			}
			break
		}
	}
	return out, nil
}

func processImagePath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	// Long paths exceed MAX_PATH, so size the buffer generously; the API
	// reports the written length either way.
	var buffer [1024]uint16
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}

// killWindowsProcessTree force-ends a process and everything it spawned.
// Terminating only the parent would leave the web server, Elasticsearch, and
// compute-engine JVMs running, which is exactly how the leaked instances
// held their locks in the first place.
func killWindowsProcessTree(ctx context.Context, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid managed SonarQube process id %d", pid)
	}
	output, err := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		text := strings.ToLower(string(output))
		if strings.Contains(text, "not found") || strings.Contains(text, "no running instance") {
			return nil
		}
		return fmt.Errorf("taskkill /T /F: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// killOnCloseJob holds the single job object that every managed server joins.
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE makes the operating system end the whole
// Java tree when this process's last job-handle closes — crucially including
// a crash or taskkill that never runs Shutdown — so server children can no
// longer outlive Blunt Code and poison the next session's startup.
var killOnCloseJob struct {
	sync.Once
	handle windows.Handle
	err    error
}

// trackProcessInKillOnCloseJob adds a freshly started server process to the
// kill-on-close job. Children the process spawns afterwards inherit the job
// automatically; the assignment races only against a child spawned in the
// microseconds between process creation and assignment, and the startup
// sweep remains the backstop for anything that slips through.
func trackProcessInKillOnCloseJob(process *os.Process) error {
	if process == nil || process.Pid <= 0 {
		return fmt.Errorf("managed SonarQube process is not running")
	}
	killOnCloseJob.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			killOnCloseJob.err = fmt.Errorf("create kill-on-close job: %w", err)
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(
			job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(job)
			killOnCloseJob.err = fmt.Errorf("configure kill-on-close job: %w", err)
			return
		}
		killOnCloseJob.handle = job
	})
	if killOnCloseJob.err != nil {
		return killOnCloseJob.err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("open managed SonarQube process %d for job assignment: %w", process.Pid, err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(killOnCloseJob.handle, handle); err != nil {
		return fmt.Errorf("assign managed SonarQube process %d to kill-on-close job: %w", process.Pid, err)
	}
	return nil
}
