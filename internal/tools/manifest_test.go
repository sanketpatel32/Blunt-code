package tools

import "testing"

func TestArtifactRejectsLatestAndMissingChecksum(t *testing.T) {
	a := Artifact{ToolID: "x", Version: "latest", Platform: "windows-amd64", SourceURL: "https://example.com/x", Executable: "x.exe"}
	if a.Validate() == nil {
		t.Fatal("latest manifest entry must be rejected")
	}
}

func TestDefaultManifestPinsManagedUVAndSemgrep(t *testing.T) {
	manifest, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}
	uv, ok := manifest.Find("uv", "windows-amd64")
	if !ok || uv.Version != "0.11.16" || uv.ArchiveType != "zip" || len(uv.SHA256) != 64 {
		t.Fatalf("unexpected uv artifact: %#v", uv)
	}
	semgrep, ok := manifest.Find("semgrep", "windows-amd64")
	if !ok || semgrep.Version != "1.172.0" || len(semgrep.SHA256) != 64 || semgrep.InstallKind != "uv_tool" || semgrep.Package != "semgrep==1.172.0" {
		t.Fatalf("unexpected Semgrep artifact: %#v", semgrep)
	}
}
