package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"bike-trip/server/internal/store"
	"bike-trip/server/internal/trip"
)

// tripPayload is everything a client needs to render a trip and keep working
// with no signal: the plan, what has been logged against it, and the cached
// road geometry. One request, one thing to put in the offline cache.
type tripPayload struct {
	ID       string           `json:"id"`
	Slug     string           `json:"slug"`
	Name     string           `json:"name"`
	Revision int              `json:"revision"`
	Access   store.Access     `json:"access"`
	Doc      json.RawMessage  `json:"doc"`
	Log      []store.LogEntry `json:"log"`
	Kit      []store.KitEntry `json:"kit"`
	Routes   []store.Route    `json:"routes"`
	// Set only for a caller holding the admin token, so the owner can find
	// the share links without a separate request.
	ViewToken string `json:"viewToken,omitempty"`
	EditToken string `json:"editToken,omitempty"`
	// Things the write did that the caller did not ask for and should know
	// about — a base moved to a different arrival day, say. Not errors: the
	// write happened.
	Warnings []trip.FieldError `json:"warnings,omitempty"`
}

func (s *Server) listTrips(w http.ResponseWriter, r *http.Request) {
	trips, err := s.store.ListTrips(r.Context())
	if err != nil {
		slog.Error("list trips", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read the trip list.")
		return
	}

	// The listing is admin-only, so the links belong in it — otherwise the
	// owner has no way back to a trip they created on another machine.
	type row struct {
		store.Summary
		ViewToken string `json:"viewToken"`
		EditToken string `json:"editToken"`
	}
	out := make([]row, 0, len(trips))
	for _, sum := range trips {
		full, err := s.store.TripByID(r.Context(), sum.ID)
		if err != nil {
			continue
		}
		out = append(out, row{Summary: sum, ViewToken: full.ViewToken, EditToken: full.EditToken})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createTrip(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.decodeAndValidate(w, r)
	if !ok {
		return
	}

	created, err := s.store.CreateTrip(r.Context(), doc)
	if errors.Is(err, store.ErrSlugTaken) {
		writeFieldErrors(w, "That slug is already in use.", []trip.FieldError{{
			Path:    "slug",
			Message: fmt.Sprintf("another trip already uses %q — pick a different one", doc.Slug),
		}}, nil)
		return
	}
	if err != nil {
		slog.Error("create trip", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save that trip.")
		return
	}

	slog.Info("trip created", "slug", created.Slug, "id", created.ID)
	writeJSON(w, http.StatusCreated, tripPayload{
		ID: created.ID, Slug: created.Slug, Name: created.Name,
		Revision: created.Revision, Access: store.AccessEdit, Doc: created.Doc,
		Log: []store.LogEntry{}, Kit: []store.KitEntry{}, Routes: []store.Route{},
		ViewToken: created.ViewToken, EditToken: created.EditToken,
	})
}

func (s *Server) getTrip(w http.ResponseWriter, r *http.Request) {
	t, access := tripFrom(r)

	logEntries, err := s.store.Log(r.Context(), t.ID)
	if err != nil {
		slog.Error("read log", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read the log.")
		return
	}
	kit, err := s.store.KitState(r.Context(), t.ID)
	if err != nil {
		slog.Error("read kit state", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read the checklist.")
		return
	}
	routes, err := s.store.Routes(r.Context(), t.ID)
	if err != nil {
		slog.Error("read routes", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read the cached routes.")
		return
	}

	payload := tripPayload{
		ID: t.ID, Slug: t.Slug, Name: t.Name, Revision: t.Revision,
		Access: access, Doc: t.Doc, Log: logEntries, Kit: kit, Routes: routes,
	}
	if s.hasAdmin(r) {
		payload.ViewToken = t.ViewToken
		payload.EditToken = t.EditToken
	}

	w.Header().Set("ETag", strconv.Itoa(t.Revision))
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) exportTrip(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	// Re-encode indented: an export is meant to be read, edited by hand or by
	// a model, and pushed back.
	var pretty json.RawMessage
	var buf []byte
	if err := json.Unmarshal(t.Doc, &pretty); err == nil {
		var v any
		if err := json.Unmarshal(t.Doc, &v); err == nil {
			buf, _ = json.MarshalIndent(v, "", "  ")
		}
	}
	if buf == nil {
		buf = t.Doc
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", t.Slug+".json"))
	w.WriteHeader(http.StatusOK)
	w.Write(append(buf, '\n'))
}

func (s *Server) replaceTrip(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	doc, ok := s.decodeAndValidate(w, r)
	if !ok {
		return
	}

	ifRevision, ok := parseIfMatch(w, r)
	if !ok {
		return
	}

	updated, err := s.store.ReplaceDoc(r.Context(), t.ID, doc, ifRevision)
	switch {
	case errors.Is(err, store.ErrRevisionMismatch):
		current, _ := s.store.TripByID(r.Context(), t.ID)
		rev := 0
		if current != nil {
			rev = current.Revision
		}
		writeJSON(w, http.StatusConflict, errorBody{
			Message:  "Someone else saved this trip while you were editing. Reload to get their version, then reapply your change.",
			Revision: rev,
		})
		return
	case errors.Is(err, store.ErrSlugTaken):
		writeFieldErrors(w, "That slug is already in use.", []trip.FieldError{{
			Path: "slug", Message: fmt.Sprintf("another trip already uses %q", doc.Slug),
		}}, nil)
		return
	case err != nil:
		slog.Error("replace trip", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save that trip.")
		return
	}

	s.respondWithTrip(w, r, updated)
}

func (s *Server) patchTrip(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	var body struct {
		Ops []trip.Op `json:"ops"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Could not read the edits: "+err.Error())
		return
	}
	if len(body.Ops) == 0 {
		writeError(w, http.StatusBadRequest, "No edits in the request.")
		return
	}

	patched, patchErrs := trip.ApplyPatch(t.Doc, body.Ops)
	if len(patchErrs) > 0 {
		writeFieldErrors(w, "Those edits do not fit the trip.", patchErrs, nil)
		return
	}

	// Revalidate: a patch must not be able to produce a document that a full
	// import would have been refused.
	var doc trip.Trip
	if err := json.Unmarshal(patched, &doc); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Those edits produced an unreadable trip.")
		return
	}
	if errs, warnings := trip.Validate(&doc); len(errs) > 0 {
		writeFieldErrors(w, "Those edits would break the trip.", errs, warnings)
		return
	}
	trip.Normalize(&doc)

	ifRevision, ok := parseIfMatch(w, r)
	if !ok {
		return
	}

	updated, err := s.store.ReplaceDoc(r.Context(), t.ID, &doc, ifRevision)
	if errors.Is(err, store.ErrRevisionMismatch) {
		current, _ := s.store.TripByID(r.Context(), t.ID)
		rev := 0
		if current != nil {
			rev = current.Revision
		}
		writeJSON(w, http.StatusConflict, errorBody{
			Message:  "Someone else saved while you were editing. Reload and try that change again.",
			Revision: rev,
		})
		return
	}
	if err != nil {
		slog.Error("patch trip", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save that edit.")
		return
	}

	s.respondWithTrip(w, r, updated)
}

func (s *Server) deleteTrip(w http.ResponseWriter, r *http.Request) {
	t, ok := s.adminTrip(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteTrip(r.Context(), t.ID); err != nil {
		slog.Error("delete trip", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not delete that trip.")
		return
	}
	slog.Info("trip deleted", "slug", t.Slug, "id", t.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rotateTokens(w http.ResponseWriter, r *http.Request) {
	t, ok := s.adminTrip(w, r)
	if !ok {
		return
	}
	updated, err := s.store.RotateTokens(r.Context(), t.ID)
	if err != nil {
		slog.Error("rotate tokens", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not issue new links.")
		return
	}
	slog.Info("tokens rotated", "slug", t.Slug)
	writeJSON(w, http.StatusOK, map[string]string{
		"viewToken": updated.ViewToken,
		"editToken": updated.EditToken,
	})
}

/* -------------------------------- helpers -------------------------------- */

// decodeAndValidate reads a trip document from the body and checks it.
// Warnings are returned alongside a successful parse, not treated as failure.
func (s *Server) decodeAndValidate(w http.ResponseWriter, r *http.Request) (*trip.Trip, bool) {
	var doc trip.Trip
	if err := decodeJSON(w, r, &doc); err != nil {
		writeFieldErrors(w, "That is not a trip document.", []trip.FieldError{{
			Path:    "",
			Message: err.Error(),
		}}, nil)
		return nil, false
	}

	errs, warnings := trip.Validate(&doc)
	if len(errs) > 0 {
		writeFieldErrors(w, fmt.Sprintf("The trip has %d problem(s).", len(errs)), errs, warnings)
		return nil, false
	}
	trip.Normalize(&doc)
	return &doc, true
}

// parseIfMatch reads the revision the client believes it is editing.
// Absent means "I have not checked" and forces the write.
func parseIfMatch(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.Header.Get("If-Match")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "If-Match must be the revision number you loaded.")
		return 0, false
	}
	return n, true
}

// respondWithTrip returns the saved trip. Warnings are variadic because almost
// no write produces any, and the ones that do should not make every other call
// site pass nil.
func (s *Server) respondWithTrip(w http.ResponseWriter, r *http.Request, t *store.Trip, warnings ...trip.FieldError) {
	logEntries, _ := s.store.Log(r.Context(), t.ID)
	kit, _ := s.store.KitState(r.Context(), t.ID)
	routes, _ := s.store.Routes(r.Context(), t.ID)

	_, access := tripFrom(r)
	w.Header().Set("ETag", strconv.Itoa(t.Revision))
	writeJSON(w, http.StatusOK, tripPayload{
		ID: t.ID, Slug: t.Slug, Name: t.Name, Revision: t.Revision,
		Access: access, Doc: t.Doc, Log: logEntries, Kit: kit, Routes: routes,
		Warnings: warnings,
	})
}
