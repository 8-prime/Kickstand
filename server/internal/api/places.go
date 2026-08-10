package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"bike-trip/server/internal/nominatim"
	"bike-trip/server/internal/trip"
)

// searchPlaces finds coordinates for a name typed into the stop editor.
//
// Edit access, not view: every call spends an upstream request against a
// service that runs on goodwill, and someone holding a read-only link has
// nothing to place.
func (s *Server) searchPlaces(w http.ResponseWriter, r *http.Request) {
	if s.nominatim == nil {
		writeError(w, http.StatusServiceUnavailable, "Place lookup is not configured on this server.")
		return
	}
	t, _ := tripFrom(r)

	q := r.URL.Query().Get("q")

	// Bias results toward the region the trip is in, so "Saint-Michel" in
	// Brittany does not come back from Quebec first.
	var near *trip.Bounds
	if doc, err := t.Document(); err == nil {
		near = &doc.Bounds
	}

	places, err := s.nominatim.Search(r.Context(), q, near)
	if err != nil {
		writePlaceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"places": places})
}

// reversePlace names the point a stop was just dropped on.
func (s *Server) reversePlace(w http.ResponseWriter, r *http.Request) {
	// Checked before the service is: whether the request makes sense does not
	// depend on whether there is a geocoder to answer it.
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, lonErr := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if latErr != nil || lonErr != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, "Give a lat and lon to look up.")
		return
	}

	if s.nominatim == nil {
		writeError(w, http.StatusServiceUnavailable, "Place lookup is not configured on this server.")
		return
	}

	place, err := s.nominatim.Reverse(r.Context(), lat, lon)
	if err != nil {
		writePlaceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"place": place})
}

// writePlaceError keeps an upstream problem out of the 500 bucket: the client
// falls back to bare coordinates on 502, which is a working stop, not a
// failure worth interrupting an edit for.
func writePlaceError(w http.ResponseWriter, err error) {
	var pe *nominatim.Error
	if errors.As(err, &pe) {
		writeError(w, http.StatusBadGateway, pe.Message)
		return
	}
	slog.Warn("place lookup aborted", "error", err)
	writeError(w, http.StatusBadGateway, "The place lookup did not finish.")
}
