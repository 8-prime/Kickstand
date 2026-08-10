// Command server runs the bike-trip API and, when one has been built into
// it, the frontend as well.
//
//	go run .                       # dev: API on :8080, Vite serves the app
//	./server -addr :8080 -db trips.db
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bike-trip/server/internal/api"
	"bike-trip/server/internal/osrm"
	"bike-trip/server/internal/store"
	"bike-trip/server/seed"
	"bike-trip/server/web"
)

func main() {
	var (
		addr        = flag.String("addr", envOr("BIKETRIP_ADDR", ":8080"), "address to listen on")
		dbPath      = flag.String("db", envOr("BIKETRIP_DB", "biketrip.db"), "path to the SQLite database")
		adminToken  = flag.String("admin-token", os.Getenv("BIKETRIP_ADMIN_TOKEN"), "token for creating, listing and deleting trips; generated if empty")
		allowOrigin = flag.String("allow-origin", os.Getenv("BIKETRIP_ALLOW_ORIGIN"), "CORS origin to allow; empty means same-origin only")
		noSeed      = flag.Bool("no-seed", false, "start with an empty database instead of loading the built-in trips")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(*addr, *dbPath, *adminToken, *allowOrigin, *noSeed); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(addr, dbPath, adminToken, allowOrigin string, noSeed bool) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !noSeed {
		if err := seedIfEmpty(ctx, st); err != nil {
			return err
		}
	}

	generated := false
	if adminToken == "" {
		adminToken, err = randomToken()
		if err != nil {
			return err
		}
		generated = true
	}

	handler := api.New(api.Options{
		Store:       st,
		OSRM:        osrm.New(),
		AdminToken:  adminToken,
		AllowOrigin: allowOrigin,
		Web:         web.Handler(),
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Routing a whole region walks the upstream router one day at a time,
		// so a refresh request can legitimately run for a while.
		WriteTimeout: 3 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	slog.Info("listening", "addr", addr, "db", dbPath)
	if generated {
		// Printed, not stored: it is needed to create or list trips, and a
		// fixed one belongs in BIKETRIP_ADMIN_TOKEN so it survives a restart.
		slog.Info("admin token generated for this run", "token", adminToken,
			"note", "set BIKETRIP_ADMIN_TOKEN to keep it across restarts")
	}

	errc := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// seedIfEmpty loads the built-in trips the first time the server runs.
//
// Only into an empty database: once you have your own trips, the seeds are
// history, and a deleted seed must stay deleted.
func seedIfEmpty(ctx context.Context, st *store.Store) error {
	n, err := st.CountTrips(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	docs, err := seed.Documents()
	if err != nil {
		return err
	}
	for _, doc := range docs {
		created, err := st.CreateTrip(ctx, doc)
		if err != nil {
			return err
		}
		slog.Info("seeded trip", "slug", created.Slug, "days", len(doc.Days))
	}
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
