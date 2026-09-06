//go:build windows

package process

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// defaultJobMemoryLimit caps the commit charge of one analyzer run's whole
// process tree (Java servers, Python wrappers, their children). 8 GiB keeps
// the heaviest legitimate run (deep-tier SonarQube) comfortable while
// stopping a runaway child from paging the machine.
const defaultJobMemoryLimit = 8 << 30

// killOnCloseJob is one job object per executed child. JOB_OBJECT_LIMIT_
// KILL_ON_JOB_CLOSE makes the operating system end everything in the job
// when the last handle closes, which happens in two load-bearing cases: the
// normal path (Run closes the handle once Wait has returned, reaping
// pipe-holding grandchildren the cancellation missed) and the crash path
// (Blunt Code itself dies, the handle closes with it, and no analyzer tree
// outlives the session).
type killOnCloseJob struct {
	handle windows.Handle
}

func newKillOnCloseJob() (*killOnCloseJob, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_JOB_MEMORY
	info.JobMemoryLimit = defaultJobMemoryLimit
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure job object: %w", err)
	}
	return &killOnCloseJob{handle: job}, nil
}

// assign adds a freshly started process to the job. Children it spawns
// afterwards inherit the job automatically; the assignment races only
// against a child spawned in the microseconds between Start and assign, and
// the taskkill /T cancellation remains the backstop for that window.
func (j *killOnCloseJob) assign(process *os.Process) error {
	if process == nil || process.Pid <= 0 {
		return fmt.Errorf("process is not running")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("open process %d for job assignment: %w", process.Pid, err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(j.handle, handle); err != nil {
		return fmt.Errorf("assign process %d to job: %w", process.Pid, err)
	}
	return nil
}

// close drops the job handle; with KILL_ON_JOB_CLOSE the operating system
// ends any process still in the job, so leftovers cannot outlive the run.
func (j *killOnCloseJob) close() {
	if j != nil && j.handle != 0 {
		_ = windows.CloseHandle(j.handle)
		j.handle = 0
	}
}
