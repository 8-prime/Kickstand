package api

import (
	"encoding/json"
	"net/http"

	"bike-trip/server/internal/trip"
	"bike-trip/server/seed"
)

// tripSchema publishes the contract for a trip document.
//
// This is the endpoint to point a model at: "generate a trip matching this
// schema". Paired with the example below it is enough to write a new trip
// without seeing any of the code.
func (s *Server) tripSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/schema+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(trip.JSONSchema)
}

// tripExample serves a complete, real document, because a schema alone
// under-specifies tone: how long a `detail` should be, what a useful `why`
// on a checklist item reads like.
func (s *Server) tripExample(w http.ResponseWriter, _ *http.Request) {
	docs, err := seed.Documents()
	if err != nil || len(docs) == 0 {
		writeError(w, http.StatusInternalServerError, "No example available.")
		return
	}

	buf, err := json.MarshalIndent(docs[0], "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not render the example.")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(append(buf, '\n'))
}
