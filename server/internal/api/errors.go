package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"bike-trip/server/internal/trip"
)

// errorBody is the one error shape this API returns.
//
// `errors` carries field-level problems with a path, which is what makes a
// rejected import fixable — by a person or by the model that wrote it.
type errorBody struct {
	Message  string            `json:"message"`
	Errors   []trip.FieldError `json:"errors,omitempty"`
	Warnings []trip.FieldError `json:"warnings,omitempty"`
	// Revision is set on a 409 so the client knows what it is behind.
	Revision int `json:"revision,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent; all that is left is to say so.
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorBody{Message: message})
}

func writeFieldErrors(w http.ResponseWriter, message string, errs, warnings []trip.FieldError) {
	writeJSON(w, http.StatusUnprocessableEntity, errorBody{
		Message:  message,
		Errors:   errs,
		Warnings: warnings,
	})
}

// decodeJSON reads a request body with a size cap and rejects unknown fields,
// so a typo in an imported document is reported rather than silently dropped.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	const maxBody = 8 << 20 // 8 MB — far above any real trip document
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
