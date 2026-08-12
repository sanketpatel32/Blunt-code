// Package secret stores small local credentials. Platform implementations
// must encrypt at rest; callers never receive a usable filename by default.
package secret

import (
	"fmt"
	"os"
	"path/filepath"
)

type Store struct{ Path string }

func (s Store) Load() ([]byte, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("credential storage path is required")
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, err
	}
	return unprotect(b)
}

func (s Store) Save(value []byte) error {
	if s.Path == "" {
		return fmt.Errorf("credential storage path is required")
	}
	b, err := protect(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp := s.Path + ".new"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
