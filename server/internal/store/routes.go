package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bike-trip/server/internal/trip"
)

// Route is one day's road geometry, as returned by the router and cached for
// everyone on the trip.
type Route struct {
	Day int `json:"day"`
	// Encoded polyline, precision 5. Compact enough to ship with the trip
	// payload and cache in the browser for offline use.
	Polyline string `json:"polyline"`
	// Road distance and driving time the router reported.
	Km    float64 `json:"km"`
	Hours float64 `json:"hours"`
	// Hash of the stops this was routed from; a day whose stops changed has a
	// stale entry and gets refetched.
	Signature string    `json:"signature"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// SignatureOf identifies a day by its stop coordinates, so editing a stop
// invalidates that day's cached geometry and nothing else.
//
// Coordinates are rounded to four decimals — about 11 metres, far below the
// precision of a hand-placed waypoint, so nudging a number in the sixth
// decimal does not throw away a good route.
func SignatureOf(stops []trip.Stop) string {
	var b strings.Builder
	for _, s := range stops {
		b.WriteString(strconv.FormatFloat(s.Lat, 'f', 4, 64))
		b.WriteByte(',')
		b.WriteString(strconv.FormatFloat(s.Lon, 'f', 4, 64))
		b.WriteByte('|')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// Routes returns every cached route for a trip.
func (s *Store) Routes(ctx context.Context, tripID string) ([]Route, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, polyline, km, hours, signature, fetched_at
		 FROM routes WHERE trip_id = ? ORDER BY day`, tripID)
	if err != nil {
		return nil, fmt.Errorf("read routes: %w", err)
	}
	defer rows.Close()

	out := []Route{}
	for rows.Next() {
		var (
			r       Route
			fetched string
		)
		if err := rows.Scan(&r.Day, &r.Polyline, &r.Km, &r.Hours, &r.Signature, &fetched); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		r.FetchedAt, _ = time.Parse(time.RFC3339, fetched)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutRoute stores or replaces a day's geometry.
func (s *Store) PutRoute(ctx context.Context, tripID string, r Route) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO routes (trip_id, day, signature, polyline, km, hours, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (trip_id, day) DO UPDATE SET
		   signature = excluded.signature, polyline = excluded.polyline,
		   km = excluded.km, hours = excluded.hours, fetched_at = excluded.fetched_at`,
		tripID, r.Day, r.Signature, r.Polyline, r.Km, r.Hours,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store route for day %d: %w", r.Day, err)
	}
	return nil
}

// StaleDays returns the routable days whose cached geometry is missing or was
// routed from different stops.
func (s *Store) StaleDays(ctx context.Context, tripID string, doc *trip.Trip) ([]trip.Day, error) {
	cached, err := s.Routes(ctx, tripID)
	if err != nil {
		return nil, err
	}
	bySignature := make(map[int]string, len(cached))
	for _, r := range cached {
		bySignature[r.Day] = r.Signature
	}

	var stale []trip.Day
	for _, d := range doc.RoutableDays() {
		if bySignature[d.N] != SignatureOf(d.Stops) {
			stale = append(stale, d)
		}
	}
	return stale, nil
}
