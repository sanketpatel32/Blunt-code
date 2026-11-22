package api

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) findingNotes(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	fingerprint := r.PathValue("fingerprint")
	if len(fingerprint) > 256 || strings.TrimSpace(fingerprint) == "" {
		fail(w, 400, "INVALID_FINGERPRINT", "A finding fingerprint is required.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		notes, err := s.db.FindingNotes(r.Context(), work.ID, fingerprint)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not load notes.")
			return
		}
		writeJSON(w, 200, map[string]any{"items": notes})
	case http.MethodPost:
		var input struct {
			Text      string `json:"text"`
			ImportKey string `json:"import_key"`
		}
		if err := decode(r, &input); err != nil {
			fail(w, 400, "INVALID_JSON", "Note text is required.")
			return
		}
		input.Text = strings.TrimSpace(input.Text)
		if len(input.ImportKey) > 256 || input.Text == "" || utf8.RuneCountInString(input.Text) > 10000 {
			fail(w, 400, "INVALID_NOTE", "Enter a note between 1 and 10,000 characters.")
			return
		}
		// Notes attach to real findings only — a phantom fingerprint would
		// silently accumulate orphans that never render anywhere.
		exists, err := s.db.FingerprintExists(r.Context(), work.ID, fingerprint)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not validate the finding.")
			return
		}
		if !exists {
			fail(w, 404, "FINDING_NOT_FOUND", "No finding in this workspace has that fingerprint.")
			return
		}
		// Unbounded notes per fingerprint let a POST loop grow the table
		// forever; the drawer then fetches the whole list on every open.
		noteCount, err := s.db.FindingNoteCount(r.Context(), work.ID, fingerprint)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not save note.")
			return
		}
		if noteCount >= 100 {
			fail(w, 409, "NOTE_LIMIT", "This finding already has 100 notes. Delete some before adding more.")
			return
		}
		note, err := s.db.AddFindingNote(r.Context(), work.ID, fingerprint, input.Text, input.ImportKey)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not save note.")
			return
		}
		writeJSON(w, 201, note)
	case http.MethodDelete:
		deleted, err := s.db.DeleteFindingNote(r.Context(), work.ID, fingerprint, r.PathValue("note"))
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not delete note.")
			return
		}
		if !deleted {
			fail(w, 404, "NOTE_NOT_FOUND", "Note was not found.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
