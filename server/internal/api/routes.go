package api

import (
	"errors"
	"log/slog"
	"net/http"

	"bike-trip/server/internal/osrm"
	"bike-trip/server/internal/store"
	"bike-trip/server/internal/trip"
)

// refreshRoutes fetches road geometry for days whose cached route is missing
// or was routed from different stops.
//
// The work happens here rather than in the browser so one fetch serves
// everyone on the trip, and so the upstream router sees a single client
// pacing itself instead of one per rider.
func (s *Server) refreshRoutes(w http.ResponseWriter, r *http.Request) {
	t, _ := tripFrom(r)

	var body struct {
		// Specific days, or empty for everything stale.
		Days []int `json:"days"`
		// Refetch even days whose cache still matches their stops.
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "Could not read the request: "+err.Error())
			return
		}
	}

	doc, err := t.Document()
	if err != nil {
		slog.Error("decode stored document", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read that trip.")
		return
	}

	todo, err := s.daysToRoute(r, t, doc, body.Days, body.Force)
	if err != nil {
		slog.Error("select days to route", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not work out what needs routing.")
		return
	}

	// Empty, not nil: a nil slice marshals as null, and the client reads this
	// as a list either way.
	failures := []trip.FieldError{}
	for _, d := range todo {
		// The client's context, so closing the tab stops the batch rather
		// than leaving it hammering the router for nobody.
		result, err := s.osrm.Route(r.Context(), d.Stops)
		if err != nil {
			var re *osrm.Error
			if errors.As(err, &re) {
				failures = append(failures, trip.FieldError{
					Path:    dayPath(doc, d.N),
					Message: re.Message,
				})
				continue
			}
			// Context cancelled or something unexpected: stop rather than
			// grinding through the rest.
			slog.Warn("routing aborted", "day", d.N, "error", err)
			break
		}

		err = s.store.PutRoute(r.Context(), t.ID, store.Route{
			Day:       d.N,
			Polyline:  result.Polyline,
			Km:        result.Km,
			Hours:     result.Hours,
			Signature: store.SignatureOf(d.Stops),
		})
		if err != nil {
			slog.Error("store route", "day", d.N, "error", err)
			failures = append(failures, trip.FieldError{
				Path:    dayPath(doc, d.N),
				Message: "routed, but could not be saved",
			})
		}
	}

	routes, err := s.store.Routes(r.Context(), t.ID)
	if err != nil {
		slog.Error("read routes", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read the routes back.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"routes":    routes,
		"attempted": len(todo),
		"failures":  failures,
	})
}

func (s *Server) daysToRoute(r *http.Request, t *store.Trip, doc *trip.Trip, days []int, force bool) ([]trip.Day, error) {
	if len(days) > 0 {
		var out []trip.Day
		for _, n := range days {
			d, ok := doc.DayByNumber(n)
			if ok && len(d.Stops) >= 2 {
				out = append(out, d)
			}
		}
		return out, nil
	}
	if force {
		return doc.RoutableDays(), nil
	}
	return s.store.StaleDays(r.Context(), t.ID, doc)
}

func dayPath(doc *trip.Trip, n int) string {
	for i, d := range doc.Days {
		if d.N == n {
			return "days[" + itoa(i) + "]"
		}
	}
	return "days"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
