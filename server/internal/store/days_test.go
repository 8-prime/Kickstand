package store

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"bike-trip/server/internal/trip"
)

// fourDays is a trip with enough days either side of a change to prove the
// renumbering moved the right rows and left the rest alone.
func fourDays(slug string) *trip.Trip {
	return &trip.Trip{
		SchemaVersion: 1,
		Slug:          slug,
		Name:          "Test " + slug,
		Bounds:        trip.Bounds{Lat0: 43, Lat1: 44, Lon0: 5, Lon1: 7},
		Bases:         []trip.Base{{Index: 1, Name: "Base", Lat: 43.5, Lon: 6, ArriveDay: 1}},
		Days: []trip.Day{
			{N: 1, Type: trip.Van, Title: "Out", Van: 900},
			{N: 2, Type: trip.Ride, Title: "Loop", Km: 200},
			{N: 3, Type: trip.Ride, Title: "Gorge", Km: 180},
			{N: 4, Type: trip.Van, Title: "Home", Van: 800},
		},
	}
}

func seedDayRows(t *testing.T, s *Store, id string) {
	t.Helper()
	ctx := context.Background()

	for day := 1; day <= 4; day++ {
		if err := s.UpsertLog(ctx, id, []LogEntry{
			{Day: day, Field: "km", Value: json.RawMessage(strconv.Itoa(day * 100)), UpdatedAt: 1},
		}); err != nil {
			t.Fatalf("seed log day %d: %v", day, err)
		}
		if err := s.PutRoute(ctx, id, Route{
			Day: day, Polyline: polylineFor(day), Km: float64(day * 100),
			Hours: 1, Signature: "sig",
		}); err != nil {
			t.Fatalf("seed route day %d: %v", day, err)
		}
	}
}

func polylineFor(day int) string {
	return string(rune('a' + day - 1))
}

func loggedKm(t *testing.T, s *Store, id string) map[int]float64 {
	t.Helper()
	entries, err := s.Log(context.Background(), id)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	out := map[int]float64{}
	for _, e := range entries {
		if e.Field != "km" {
			continue
		}
		var km float64
		if err := json.Unmarshal(e.Value, &km); err != nil {
			t.Fatalf("day %d logged %q, which is not a number: %v", e.Day, e.Value, err)
		}
		out[e.Day] = km
	}
	return out
}

func routePolylines(t *testing.T, s *Store, id string) map[int]string {
	t.Helper()
	routes, err := s.Routes(context.Background(), id)
	if err != nil {
		t.Fatalf("read routes: %v", err)
	}
	out := map[int]string{}
	for _, r := range routes {
		out[r.Day] = r.Polyline
	}
	return out
}

// The point of the whole endpoint: a day that moves takes its odometer reading
// and its cached road with it, rather than handing them to whichever day
// inherits its number.
func TestRemapCarriesLogAndRoutesWithTheirDays(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, err := s.CreateTrip(ctx, fourDays("remap"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedDayRows(t, s, created.ID)

	// Delete day 2: days 3 and 4 become 2 and 3.
	doc := fourDays("remap")
	doc.Days = []trip.Day{
		{N: 1, Type: trip.Van, Title: "Out", Van: 900},
		{N: 2, Type: trip.Ride, Title: "Gorge", Km: 180},
		{N: 3, Type: trip.Van, Title: "Home", Van: 800},
	}
	remap := map[int]int{1: 1, 2: 0, 3: 2, 4: 3}

	if _, err := s.ReplaceDocAndRemap(ctx, created.ID, doc, created.Revision, remap); err != nil {
		t.Fatalf("replace and remap: %v", err)
	}

	wantKm := map[int]float64{1: 100, 2: 300, 3: 400}
	got := loggedKm(t, s, created.ID)
	if len(got) != len(wantKm) {
		t.Fatalf("log has %d km entries, want %d: %v", len(got), len(wantKm), got)
	}
	for day, km := range wantKm {
		if got[day] != km {
			t.Errorf("day %d logged %g km, want %g — the reading followed the wrong day", day, got[day], km)
		}
	}

	wantRoutes := map[int]string{1: "a", 2: "c", 3: "d"}
	routes := routePolylines(t, s, created.ID)
	if len(routes) != len(wantRoutes) {
		t.Fatalf("routes = %v, want %d entries", routes, len(wantRoutes))
	}
	for day, line := range wantRoutes {
		if routes[day] != line {
			t.Errorf("day %d has route %q, want %q", day, routes[day], line)
		}
	}
}

// Inserting shifts days upward, which is where an in-place update would land a
// row on top of one that has not moved yet.
func TestRemapHandlesAnUpwardShift(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, err := s.CreateTrip(ctx, fourDays("insert"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedDayRows(t, s, created.ID)

	doc := fourDays("insert")
	doc.Days = append(doc.Days, trip.Day{N: 5, Type: trip.Ride, Title: "Extra"})
	// A day inserted after 1: everything from 2 up moves one later.
	remap := map[int]int{1: 1, 2: 3, 3: 4, 4: 5}

	if _, err := s.ReplaceDocAndRemap(ctx, created.ID, doc, created.Revision, remap); err != nil {
		t.Fatalf("replace and remap: %v", err)
	}

	want := map[int]float64{1: 100, 3: 200, 4: 300, 5: 400}
	got := loggedKm(t, s, created.ID)
	if len(got) != len(want) {
		t.Fatalf("log = %v, want %v", got, want)
	}
	for day, km := range want {
		if got[day] != km {
			t.Errorf("day %d logged %g km, want %g", day, got[day], km)
		}
	}
	// Day 2 is the new one and has nothing logged against it yet.
	if _, taken := got[2]; taken {
		t.Errorf("the inserted day inherited a log entry: %v", got)
	}
}

func TestRemapIsRolledBackWithAFailedWrite(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, err := s.CreateTrip(ctx, fourDays("guard"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	seedDayRows(t, s, created.ID)

	// A stale revision: the document write is refused, and the day rows must
	// not have moved on their own.
	_, err = s.ReplaceDocAndRemap(ctx, created.ID, fourDays("guard"), created.Revision+7,
		map[int]int{1: 4, 2: 3, 3: 2, 4: 1})
	if err == nil {
		t.Fatal("a stale revision should be refused")
	}

	got := loggedKm(t, s, created.ID)
	for day := 1; day <= 4; day++ {
		if got[day] != float64(day*100) {
			t.Fatalf("day %d logged %g km after a refused write, want %g", day, got[day], float64(day*100))
		}
	}
}
