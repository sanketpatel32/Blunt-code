//go:build !windows

package instance

import "errors"

var ErrAlreadyRunning = errors.New("Blunt Code is already running for this data directory")

// Guard is a no-op outside Windows. Blunt Code's supported desktop target is
// Windows, while this fallback keeps cross-platform package tests buildable.
type Guard struct{}

func Acquire(string) (*Guard, error) { return &Guard{}, nil }
func (*Guard) Close() error          { return nil }
