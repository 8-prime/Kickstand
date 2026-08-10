// Package api is the HTTP layer: routing, access control, and the JSON
// shapes the browser talks to.
package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"bike-trip/server/internal/nominatim"
	"bike-trip/server/internal/osrm"
	"bike-trip/server/internal/store"
)

// Server holds what the handlers need.
type Server struct {
	store *store.Store
	osrm  *osrm.Client
	// nominatim is optional: without it the map still works, you just place
	// stops by dragging rather than by name.
	nominatim *nominatim.Client

	// adminToken gates the operations that are not about one shared trip:
	// listing every trip, creating, deleting, and rotating share links.
	//
	// Without it, share tokens would be decoration — anyone able to reach the
	// server could list the trips and read every token from the listing. One
	// shared secret is the smallest thing that makes the links mean something,
	// and it avoids a user table nobody asked for.
	adminToken string

	// web serves the built frontend, or nil in development where Vite does.
	web http.Handler
}

// Options configures a Server.
type Options struct {
	Store      *store.Store
	OSRM       *osrm.Client
	Nominatim  *nominatim.Client
	AdminToken string
	Web        http.Handler
	// AllowOrigin enables CORS for a single origin. Empty means same-origin
	// only, which is what the Vite proxy and the embedded build both are.
	AllowOrigin string
}

// New builds the HTTP handler.
func New(o Options) http.Handler {
	s := &Server{
		store:      o.Store,
		osrm:       o.OSRM,
		nominatim:  o.Nominatim,
		adminToken: o.AdminToken,
		web:        o.Web,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/schema/trip.json", s.tripSchema)
	mux.HandleFunc("GET /api/schema/example.json", s.tripExample)

	// Admin: about the collection, not about one trip.
	mux.HandleFunc("GET /api/trips", s.admin(s.listTrips))
	mux.HandleFunc("POST /api/trips", s.admin(s.createTrip))
	mux.HandleFunc("DELETE /api/trips/{token}", s.admin(s.deleteTrip))
	mux.HandleFunc("POST /api/trips/{token}/tokens/rotate", s.admin(s.rotateTokens))

	// Everything else is addressed by a share token, which also says what you
	// are allowed to do with it.
	mux.HandleFunc("GET /api/trips/{token}", s.withTrip(store.AccessView, s.getTrip))
	mux.HandleFunc("GET /api/trips/{token}/export", s.withTrip(store.AccessView, s.exportTrip))
	mux.HandleFunc("PUT /api/trips/{token}", s.withTrip(store.AccessEdit, s.replaceTrip))
	mux.HandleFunc("PATCH /api/trips/{token}", s.withTrip(store.AccessEdit, s.patchTrip))
	mux.HandleFunc("PUT /api/trips/{token}/log", s.withTrip(store.AccessEdit, s.putLog))
	mux.HandleFunc("DELETE /api/trips/{token}/log", s.withTrip(store.AccessEdit, s.clearLog))
	mux.HandleFunc("PUT /api/trips/{token}/kit", s.withTrip(store.AccessEdit, s.putKit))
	mux.HandleFunc("DELETE /api/trips/{token}/kit", s.withTrip(store.AccessEdit, s.clearKit))
	mux.HandleFunc("POST /api/trips/{token}/routes/refresh", s.withTrip(store.AccessEdit, s.refreshRoutes))

	// Adding, removing and moving a day renumbers every day after it, which
	// the log and the route cache are keyed by — so it cannot be a patch.
	mux.HandleFunc("POST /api/trips/{token}/days", s.withTrip(store.AccessEdit, s.editDays))

	// Geocoding. Edit access: each call spends an upstream request.
	mux.HandleFunc("GET /api/trips/{token}/places", s.withTrip(store.AccessEdit, s.searchPlaces))
	mux.HandleFunc("GET /api/trips/{token}/places/reverse", s.withTrip(store.AccessEdit, s.reversePlace))

	// Anything not under /api is the single-page app.
	if s.web != nil {
		mux.Handle("/", s.web)
	}

	var h http.Handler = mux
	h = cors(o.AllowOrigin, h)
	h = requestLog(h)
	h = recoverPanics(h)
	return h
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ----------------------------- access control ---------------------------- */

type ctxKey int

const (
	ctxTrip ctxKey = iota
	ctxAccess
)

// tripFrom returns the trip resolved by withTrip.
func tripFrom(r *http.Request) (*store.Trip, store.Access) {
	t, _ := r.Context().Value(ctxTrip).(*store.Trip)
	a, _ := r.Context().Value(ctxAccess).(store.Access)
	return t, a
}

// withTrip resolves {token} to a trip and enforces the access it grants.
//
// The token is the address: there is no separate id in the URL, so a link is
// the whole credential and nothing leaks an identifier you could probe.
func (s *Server) withTrip(need store.Access, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")

		t, access, err := s.store.TripByToken(r.Context(), token)
		if errors.Is(err, store.ErrNotFound) {
			// Same answer for a wrong token and a deleted trip: a prober
			// learns nothing either way.
			writeError(w, http.StatusNotFound, "No trip for that link. Check it was copied whole.")
			return
		}
		if err != nil {
			slog.Error("resolve trip token", "error", err)
			writeError(w, http.StatusInternalServerError, "Could not read that trip.")
			return
		}

		// The admin token is the owner's master key; it edits anything.
		if s.hasAdmin(r) {
			access = store.AccessEdit
		}
		if need.CanEdit() && !access.CanEdit() {
			writeError(w, http.StatusForbidden,
				"That link is read-only. You need the edit link to change anything.")
			return
		}

		ctx := context.WithValue(r.Context(), ctxTrip, t)
		ctx = context.WithValue(ctx, ctxAccess, access)
		next(w, r.WithContext(ctx))
	}
}

// admin gates the operations that are about the whole collection.
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.hasAdmin(r) {
			writeError(w, http.StatusUnauthorized,
				"This needs the server's admin token. It is printed in the server log at startup.")
			return
		}
		next(w, r)
	}
}

func (s *Server) hasAdmin(r *http.Request) bool {
	if s.adminToken == "" {
		return false
	}
	given := r.Header.Get("X-Admin-Token")
	// Constant time: the comparison should not leak how much of the token
	// was right.
	return subtle.ConstantTimeCompare([]byte(given), []byte(s.adminToken)) == 1
}

// adminTrip resolves {token} for an admin-gated route, where access is
// already established.
func (s *Server) adminTrip(w http.ResponseWriter, r *http.Request) (*store.Trip, bool) {
	t, _, err := s.store.TripByToken(r.Context(), r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No trip for that link.")
		return nil, false
	}
	if err != nil {
		slog.Error("resolve trip token", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not read that trip.")
		return nil, false
	}
	return t, true
}

/* ------------------------------- middleware ------------------------------ */

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic serving request", "path", r.URL.Path, "value", v)
				writeError(w, http.StatusInternalServerError, "Something broke on the server.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static asset requests would drown out anything worth reading.
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			// Path only: the share token lives in it, so it is redacted below.
			"path", redactToken(r.URL.Path),
			"status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}

// redactToken keeps share tokens out of the log, where they would otherwise
// sit in plain text as a permanent credential leak.
func redactToken(path string) string {
	const prefix = "/api/trips/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := path[len(prefix):]
	if rest == "" {
		return path
	}
	token, tail, _ := strings.Cut(rest, "/")
	if len(token) < 8 {
		return path
	}
	redacted := prefix + token[:4] + "…"
	if tail != "" {
		redacted += "/" + tail
	}
	return redacted
}

func cors(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token, If-Match")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
