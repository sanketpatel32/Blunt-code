package sonarqube

import "strings"

// processSnapshot is one entry of a system process listing: the process id,
// its parent as recorded at snapshot time, and the full executable image path
// when it can be resolved.
type processSnapshot struct {
	PID, PPID int
	Image     string
}

// maxAncestryDepth bounds the ancestor walk. Real process trees are shallow;
// the cap only guarantees termination when a corrupted snapshot (for example
// a parent cycle after process-id reuse) would otherwise loop forever.
const maxAncestryDepth = 128

// ownedBySelf reports whether pid's ancestor chain, as recorded in the
// snapshot, reaches selfPID. A chain that ends at a process missing from the
// snapshot never counts as owned: that ancestor is dead, so the examined
// process is orphaned rather than part of the caller's own live tree.
func ownedBySelf(procs []processSnapshot, pid, selfPID int) bool {
	if pid == selfPID {
		return true
	}
	byPID := make(map[int]processSnapshot, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	current := pid
	for depth := 0; depth < maxAncestryDepth; depth++ {
		parent, ok := byPID[current]
		if !ok || parent.PPID <= 0 {
			return false
		}
		if parent.PPID == selfPID {
			return true
		}
		current = parent.PPID
	}
	return false
}

// selectStrayServerProcesses returns the ids of processes running exactly the
// managed server executable that are not part of the caller's own live tree.
// Those leftovers come from a previous Blunt Code session whose server
// supervisor died first: Windows never ends a child when its parent exits, so
// the orphaned JVMs keep running and hold locks over the shared managed
// runtime (deployed plugin jars, the embedded database) that fail every later
// server boot. Matching is a full case-insensitive image-path comparison, so
// an unrelated java.exe is never touched, and processes reachable from
// selfPID (the current server tree, or an in-flight sonar-scanner) are spared.
func selectStrayServerProcesses(procs []processSnapshot, executable string, selfPID int) []int {
	if executable == "" {
		return nil
	}
	var strays []int
	for _, p := range procs {
		if p.PID == selfPID || !strings.EqualFold(p.Image, executable) {
			continue
		}
		if ownedBySelf(procs, p.PID, selfPID) {
			continue
		}
		strays = append(strays, p.PID)
	}
	return strays
}
