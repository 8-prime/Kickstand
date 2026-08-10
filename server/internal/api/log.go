package api

import (
	"log/slog"
	"net/http"

	"bike-trip/server/internal/store"
)

// putLog applies a batch of log writes.
//
// This is what an offline client flushes when it reconnects, so it has to be
// safe to send twice and safe to send out of order. The store settles both by
// comparing updatedAt per field.
func (s *Server) putLog(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	var body struct {
		Entries []store.LogEntry `json:"entries"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Could not read the log entries: "+err.Error())
		return
	}

	for _, e := range body.Entries {
		if !store.ValidLogField(e.Field) {
			writeError(w, http.StatusBadRequest,
				"The log holds km, wx and note. It does not hold "+e.Field+".")
			return
		}
		if e.Day <= 0 {
			writeError(w, http.StatusBadRequest, "Every log entry needs a day number.")
			return
		}
	}

	if err := s.store.UpsertLog(r.Context(), t.ID, body.Entries); err != nil {
		slog.Error("write log", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save the log.")
		return
	}

	entries, err := s.store.Log(r.Context(), t.ID)
	if err != nil {
		slog.Error("read log", "error", err)
		writeError(w, http.StatusInternalServerError, "Saved, but could not read the log back.")
		return
	}
	// Return the settled state: a client that lost a field to a newer write
	// sees the winner immediately rather than believing its own value.
	writeJSON(w, http.StatusOK, map[string]any{"log": entries})
}

func (s *Server) clearLog(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)
	if err := s.store.ClearLog(r.Context(), t.ID); err != nil {
		slog.Error("clear log", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not clear the log.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": []store.LogEntry{}})
}

func (s *Server) putKit(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	var body struct {
		Entries []store.KitEntry `json:"entries"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Could not read the checklist changes: "+err.Error())
		return
	}
	for _, e := range body.Entries {
		if e.ItemID == "" {
			writeError(w, http.StatusBadRequest, "Every checklist change needs an item id.")
			return
		}
	}

	if err := s.store.UpsertKit(r.Context(), t.ID, body.Entries); err != nil {
		slog.Error("write kit state", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save the checklist.")
		return
	}

	kit, err := s.store.KitState(r.Context(), t.ID)
	if err != nil {
		slog.Error("read kit state", "error", err)
		writeError(w, http.StatusInternalServerError, "Saved, but could not read the checklist back.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kit": kit})
}

func (s *Server) clearKit(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)
	if err := s.store.ClearKit(r.Context(), t.ID); err != nil {
		slog.Error("clear kit state", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not clear the checklist.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kit": []store.KitEntry{}})
}
