package database

import (
	"bluntcode/internal/core"
	"context"
	"path/filepath"
	"testing"
)

func TestFindingNotesPersistAndAreScopedToWorkspace(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "notes.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Notes", RootPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Other", RootPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	note, err := db.AddFindingNote(ctx, work.ID, "same-fingerprint", "Review before release")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := db.AddFindingNote(ctx, work.ID, "same-fingerprint", "Browser note", "legacy-1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.AddFindingNote(ctx, work.ID, "same-fingerprint", "Browser note", "legacy-1")
	if err != nil || again.ID != imported.ID {
		t.Fatalf("idempotent import = %+v %v", again, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	notes, err := db.FindingNotes(ctx, work.ID, "same-fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Text != note.Text {
		t.Fatalf("persisted notes = %+v", notes)
	}
	deleted, err := db.DeleteFindingNote(ctx, other.ID, "same-fingerprint", note.ID)
	if err != nil || deleted {
		t.Fatalf("cross-workspace deletion = %v, %v", deleted, err)
	}
	deleted, err = db.DeleteFindingNote(ctx, work.ID, "same-fingerprint", note.ID)
	if err != nil || !deleted {
		t.Fatalf("deletion = %v, %v", deleted, err)
	}
}
