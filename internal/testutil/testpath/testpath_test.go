package testpath

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNative(t *testing.T) {
	for in, unix := range map[string]string{
		`C:\ws\main.py`:      "/ws/main.py",
		`C:\\source\\app.py`: "/source/app.py",
		`C:\ws`:              "/ws",
		`C:\`:                "/",
	} {
		got := Native(in)
		if runtime.GOOS == "windows" {
			if got != in {
				t.Fatalf("Native(%q) = %q, want it unchanged on Windows", in, got)
			}
			continue
		}
		if got != unix {
			t.Fatalf("Native(%q) = %q, want %q", in, got, unix)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("Native(%q) = %q is not absolute", in, got)
		}
	}
}
