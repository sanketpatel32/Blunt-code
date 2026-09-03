package scans

import (
	"reflect"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestSeverityCounts(t *testing.T) {
	findings := []analyzers.Finding{
		{Severity: analyzers.SeverityCritical},
		{Severity: analyzers.SeverityCritical},
		{Severity: analyzers.SeverityLow},
		{Severity: analyzers.SeverityInfo},
	}
	got := severityCounts(findings)
	want := map[string]int{"critical": 2, "low": 1, "info": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("severityCounts = %v, want %v", got, want)
	}
	empty := severityCounts(nil)
	if empty == nil || len(empty) != 0 {
		t.Fatalf("severityCounts(nil) = %v, want empty non-nil map", empty)
	}
}
