package api

import (
	"errors"
	"log/slog"
	"net/http"

	"bike-trip/server/internal/store"
	"bike-trip/server/internal/trip"
)

// editDays adds, removes or moves a day.
//
// This is its own endpoint rather than a patch on `days` because day numbers
// are keys, not just labels: the log and the route cache are both stored
// against them. A client that set the whole day list would renumber the
// document and leave every rider's odometer reading attached to whichever day
// inherited its number. Doing it here keeps the renumbering and the key move
// in one transaction.
func (s *Server) editDays(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	var op trip.DayOp
	if err := decodeJSON(w, r, &op); err != nil {
		writeError(w, http.StatusBadRequest, "Could not read that change: "+err.Error())
		return
	}

	doc, err := t.Document()
	if err != nil {
		slog.Error("decode stored document", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read that trip.")
		return
	}

	remap, warnings, err := trip.ApplyDayOp(doc, op)
	if err != nil {
		writeError(w, http.StatusBadRequest, capitalise(err.Error())+".")
		return
	}

	// Same rule as a patch: a structural change must not be able to produce a
	// document that a full import would have been refused.
	if errs, more := trip.Validate(doc); len(errs) > 0 {
		writeFieldErrors(w, "That change would break the trip.", errs, append(warnings, more...))
		return
	}
	trip.Normalize(doc)

	ifRevision, ok := parseIfMatch(w, r)
	if !ok {
		return
	}

	updated, err := s.store.ReplaceDocAndRemap(r.Context(), t.ID, doc, ifRevision, remap)
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
		slog.Error("edit days", "op", op.Op, "error", err)
		writeError(w, http.StatusInternalServerError, "Could not save that change.")
		return
	}

	slog.Info("days edited", "slug", t.Slug, "op", op.Op, "days", len(doc.Days))
	s.respondWithTrip(w, r, updated, warnings...)
}

func capitalise(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-32) + s[1:]
}
