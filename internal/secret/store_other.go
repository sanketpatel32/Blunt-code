//go:build !windows

package secret

import "fmt"

func protect([]byte) ([]byte, error) {
	return nil, fmt.Errorf("credential storage is only supported on Windows")
}
func unprotect([]byte) ([]byte, error) {
	return nil, fmt.Errorf("credential storage is only supported on Windows")
}
