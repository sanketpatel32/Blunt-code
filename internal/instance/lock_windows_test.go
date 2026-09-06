//go:build windows

package instance

import (
	"errors"
	"strings"
	"testing"
)

// The mutex must live in the Global namespace: a Local\ mutex is invisible to
// other Windows sessions, so the same user could run two backends on the same
// user-local database from two sessions. The name is derived from the data
// directory, so the test asserts the prefix rather than a fixed string.
func TestMutexNameIsMachineWidePerDataDir(t *testing.T) {
	if name := mutexName(`C:\Users\A\AppData\Local\BluntCode`); !strings.HasPrefix(name, `Global\BluntCode-`) {
		t.Fatalf("mutex name %q is not in the machine-wide namespace", name)
	}
	if mutexName(`C:\Users\A\AppData\Local\BluntCode`) != mutexName(`c:\users\a\appdata\local\bluntcode\`) {
		t.Fatal("mutex name must be canonical across case and trailing separators")
	}
	if mutexName(`C:\Data\A`) == mutexName(`C:\Data\B`) {
		t.Fatal("distinct data directories must map to distinct mutexes")
	}
}

func TestAcquirePreventsSecondBackendForSameDataDir(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := Acquire(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want ErrAlreadyRunning", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(dir)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	defer second.Close()
}
