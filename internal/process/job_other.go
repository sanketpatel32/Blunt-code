//go:build !windows

package process

import "os"

// killOnCloseJob is the Windows job-object supervision reduced to a no-op:
// elsewhere, cancelling the command context signals the direct child and
// process groups keep the tree story honest enough for the analyzers run
// here. The type exists so Run's supervision path stays single-shaped.
type killOnCloseJob struct{}

func newKillOnCloseJob() (*killOnCloseJob, error) { return &killOnCloseJob{}, nil }
func (j *killOnCloseJob) assign(*os.Process) error { return nil }
func (j *killOnCloseJob) close()                    {}
