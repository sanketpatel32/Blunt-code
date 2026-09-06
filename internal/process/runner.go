// Package process executes analyzers with argument arrays, bounded output and cancellation.
package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultOutputLimit = 8 << 20

// cancelGrace bounds how long cmd.Wait keeps waiting after the context is
// cancelled: a killed analyzer child may leave grandchildren (JVMs, Python
// wrappers) holding the inherited output pipes open, and without this bound a
// "10 minute" analyzer timeout would block the scan far beyond it.
const cancelGrace = 5 * time.Second

type Request struct {
	Command     string
	Args        []string
	Dir         string
	Env         []string
	OutputLimit int
	OnOutput    func(stream, line string)
}
type Result struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Duration  time.Duration
	Truncated bool
}

func Run(ctx context.Context, req Request) (Result, error) {
	if req.Command == "" {
		return Result{}, fmt.Errorf("command is required")
	}
	limit := req.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	cmd.Dir = req.Dir
	if req.Env != nil {
		cmd.Env = req.Env
	}
	// Killing only the direct child orphans its descendants on Windows, so
	// cancellation terminates the whole process tree. Best effort: errors are
	// swallowed so a race with natural process exit never fails a successful run.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = terminateTree(cmd.Process.Pid)
		}
		return nil
	}
	cmd.WaitDelay = cancelGrace
	out := &CappedBuffer{limit: limit, onWrite: func(p []byte) {
		if req.OnOutput != nil {
			req.OnOutput("stdout", string(p))
		}
	}}
	errout := &CappedBuffer{limit: limit, onWrite: func(p []byte) {
		if req.OnOutput != nil {
			req.OnOutput("stderr", string(p))
		}
	}}
	cmd.Stdout = out
	cmd.Stderr = errout
	// Every executed child joins a kill-on-close job object for its whole
	// life: descendants that outlive cancellation (pipe-holding
	// grandchildren) are ended by the operating system when the job handle
	// closes after Wait, and a crash of Blunt Code itself takes the entire
	// analyzer tree down instead of leaking it onto the machine. Job
	// supervision is best effort — if the platform refuses it, the
	// taskkill/tree cancellation below remains the containment story.
	job, jobErr := newKillOnCloseJob()
	if jobErr == nil {
		defer job.close()
	}
	started := time.Now()
	err := cmd.Start()
	if err != nil {
		return Result{}, err
	}
	if jobErr == nil {
		if err := job.assign(cmd.Process); err != nil {
			// The window between Start and assign can leak the child out of
			// the job; end it now rather than run unsupervised.
			_ = terminateTree(cmd.Process.Pid)
			_ = cmd.Wait()
			return Result{}, fmt.Errorf("supervise process: %w", err)
		}
	}
	err = cmd.Wait()
	result := Result{Stdout: out.Bytes(), Stderr: errout.Bytes(), Duration: time.Since(started), Truncated: out.truncated || errout.truncated}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// terminateTree ends a child process and its descendants. On Windows,
// Process.Kill terminates only the direct process, so taskkill /T /F is used
// to take spawned analyzers (uv, Java) with it; elsewhere the plain kill is
// the available contract.
func terminateTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid process id %d", pid)
	}
	if runtime.GOOS != "windows" {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return process.Kill()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
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

// CappedBuffer is a byte buffer that stops retaining output once it holds
// limit bytes, while still reporting writes as successful so a flooding
// child never blocks or fails its run. It is safe for concurrent use, so one
// buffer can serve as both Stdout and Stderr of a single command.
type CappedBuffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	limit     int
	truncated bool
	onWrite   func([]byte)
}

// NewCappedBuffer returns a CappedBuffer that retains at most limit bytes.
func NewCappedBuffer(limit int) *CappedBuffer {
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	return &CappedBuffer{limit: limit}
}

// Truncated reports whether any output was discarded.
func (l *CappedBuffer) Truncated() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.truncated
}

// Bytes returns a copy of the retained output.
func (l *CappedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.b.Bytes()...)
}

func (l *CappedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.onWrite != nil {
		l.onWrite(p)
	}
	remaining := l.limit - l.b.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = l.b.Write(p[:remaining])
		l.truncated = true
		return len(p), nil
	}
	_, _ = l.b.Write(p)
	return len(p), nil
}
