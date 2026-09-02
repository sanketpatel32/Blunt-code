package sonarqube

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectStrayServerProcesses(t *testing.T) {
	const managedJava = `C:\BluntCode\tools\java\21\bin\java.exe`
	const self = 100
	// 90 (alive ancestor) → 100 self → 101 live server App → 102 its child:
	// the caller's own tree, never a stray even though it runs the managed
	// java.exe. 199 is absent from the snapshot (dead), so 200 and 201 are
	// orphans from a previous session — exactly the 2026-09-02 leak shape.
	procs := []processSnapshot{
		{PID: 90, PPID: 80, Image: `C:\Windows\explorer.exe`},
		{PID: 100, PPID: 90, Image: `C:\BluntCode\bluntcode.exe`},
		{PID: 101, PPID: 100, Image: managedJava},
		{PID: 102, PPID: 101, Image: managedJava},
		{PID: 103, PPID: 101, Image: `C:\Windows\System32\other.exe`},
		{PID: 200, PPID: 199, Image: managedJava},
		{PID: 201, PPID: 200, Image: managedJava},
		{PID: 300, PPID: 299, Image: `C:\Program Files\other\java.exe`},
		{PID: 301, PPID: 299, Image: strings.ToLower(managedJava)},
	}
	strays := selectStrayServerProcesses(procs, managedJava, self)
	if !reflect.DeepEqual(strays, []int{200, 201, 301}) {
		t.Fatalf("strays = %v, want [200 201 301] (301 matches case-insensitively)", strays)
	}
}

func TestSelectStrayServerProcessesRequiresExecutable(t *testing.T) {
	if got := selectStrayServerProcesses([]processSnapshot{{PID: 1, Image: "x"}}, "", 2); got != nil {
		t.Fatalf("empty executable must select nothing, got %v", got)
	}
}

func TestOwnedBySelfTerminatesOnParentCycle(t *testing.T) {
	// A corrupted snapshot (pid reuse) must not hang the ancestor walk.
	cycled := []processSnapshot{
		{PID: 10, PPID: 11, Image: "a.exe"},
		{PID: 11, PPID: 10, Image: "b.exe"},
	}
	if ownedBySelf(cycled, 10, 99) {
		t.Fatal("cycle must not report the process as owned")
	}
}
