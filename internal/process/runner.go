// Package process executes analyzers with argument arrays, bounded output and cancellation.
package process

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

const DefaultOutputLimit = 8 << 20

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
	out := &limitedBuffer{limit: limit, onWrite: func(p []byte) {
		if req.OnOutput != nil {
			req.OnOutput("stdout", string(p))
		}
	}}
	errout := &limitedBuffer{limit: limit, onWrite: func(p []byte) {
		if req.OnOutput != nil {
			req.OnOutput("stderr", string(p))
		}
	}}
	cmd.Stdout = out
	cmd.Stderr = errout
	started := time.Now()
	err := cmd.Run()
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

type limitedBuffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	limit     int
	truncated bool
	onWrite   func([]byte)
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
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
func (l *limitedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]byte(nil), l.b.Bytes()...)
}
