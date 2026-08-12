//go:build !windows

package doctor

import "fmt"

func freeSpace(string) (uint64, error) {
	return 0, fmt.Errorf("disk-space diagnostics are only available on Windows")
}
