package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"bike-trip/server/internal/trip"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func doc(slug string) *trip.Trip {
	return &trip.Trip{
		SchemaVersion: 1,
		Slug:          slug,
		Name:          "Test " + slug,
		Bounds:        trip.Bounds{Lat0: 43, Lat1: 44, Lon0: 5, Lon1: 7},
		Bases:         []trip.Base{{Index: 1, Name: "Base", Lat: 43.5, Lon: 6, ArriveDay: 1}},
		Days: []trip.Day{
			{N: 1, Type: trip.Van, Title: "Out", Van: 900},
			{N: 2, Type: trip.Ride, Title: "Loop", Km: 200, Stops: []trip.Stop{
				{Name: "A", Lat: 43.5, Lon: 6},
				{Name: "B", Lat: 43.6, Lon: 6.2},
			}},
		},
	}
}

func TestCreateAndResolveTokens(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, err := s.CreateTrip(ctx, doc("alps"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ViewToken == created.EditToken {
		t.Fatal("view and edit tokens must differ")
	}

	got, access, err := s.TripByToken(ctx, created.ViewToken)
	if err != nil || access != AccessView {
		t.Fatalf("view token: access=%q err=%v", access, err)
	}
	if got.ID != created.ID {
		t.Errorf("wrong trip: %s", got.ID)
	}
	if access.CanEdit() {
		t.Error("a view token must not grant edit")
	}

	_, access, err = s.TripByToken(ctx, created.EditToken)
	if err != nil || !access.CanEdit() {
		t.Fatalf("edit token: access=%q err=%v", access, err)
	}

	if _, _, err := s.TripByToken(ctx, "not-a-token"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token should be ErrNotFound, got %v", err)
	}
}

func TestSlugMustBeUnique(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	if _, err := s.CreateTrip(ctx, doc("alps")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTrip(ctx, doc("alps")); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("expected ErrSlugTaken, got %v", err)
	}
}

// The lost-update guard: two people editing from the same starting revision,
// the second must be told rather than silently overwriting the first.
func TestReplaceDocRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, err := s.CreateTrip(ctx, doc("alps"))
	if err != nil {
		t.Fatal(err)
	}

	updated := doc("alps")
	updated.Name = "First writer"
	after, err := s.ReplaceDoc(ctx, created.ID, updated, created.Revision)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if after.Revision != created.Revision+1 {
		t.Errorf("revision did not advance: %d", after.Revision)
	}

	second := doc("alps")
	second.Name = "Second writer"
	_, err = s.ReplaceDoc(ctx, created.ID, second, created.Revision) // stale
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", err)
	}

	current, _ := s.TripByID(ctx, created.ID)
	if current.Name != "First writer" {
		t.Errorf("stale write got through: %q", current.Name)
	}

	// Revision 0 means "I know, overwrite anyway".
	if _, err := s.ReplaceDoc(ctx, created.ID, second, 0); err != nil {
		t.Fatalf("forced write: %v", err)
	}
}

// Replacing the plan must not throw away what people wrote against it.
func TestReplaceDocKeepsLogAndTokens(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	created, _ := s.CreateTrip(ctx, doc("alps"))
	if err := s.UpsertLog(ctx, created.ID, []LogEntry{
		{Day: 2, Field: "km", Value: json.RawMessage(`271`), UpdatedAt: 1000},
	}); err != nil {
		t.Fatal(err)
	}

	replaced := doc("alps")
	replaced.Name = "Rewritten"
	after, err := s.ReplaceDoc(ctx, created.ID, replaced, created.Revision)
	if err != nil {
		t.Fatal(err)
	}

	if after.ViewToken != created.ViewToken || after.EditToken != created.EditToken {
		t.Error("tokens changed on a document replace; share links would break")
	}
	entries, _ := s.Log(ctx, created.ID)
	if len(entries) != 1 || string(entries[0].Value) != "271" {
		t.Errorf("log lost on replace: %v", entries)
	}
}

// An offline client flushes a batch it may already have sent. Replaying must
// be a no-op, and an older entry must never beat a newer one.
func TestLogIsLastWriteWinsAndIdempotent(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	created, _ := s.CreateTrip(ctx, doc("alps"))

	newer := []LogEntry{{Day: 2, Field: "km", Value: json.RawMessage(`300`), UpdatedAt: 2000}}
	older := []LogEntry{{Day: 2, Field: "km", Value: json.RawMessage(`100`), UpdatedAt: 1000}}

	if err := s.UpsertLog(ctx, created.ID, newer); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertLog(ctx, created.ID, older); err != nil {
		t.Fatal(err)
	}

	entries, _ := s.Log(ctx, created.ID)
	if len(entries) != 1 || string(entries[0].Value) != "300" {
		t.Fatalf("older write won: %v", entries)
	}

	// Replay the same batch; nothing should change or duplicate.
	if err := s.UpsertLog(ctx, created.ID, newer); err != nil {
		t.Fatal(err)
	}
	entries, _ = s.Log(ctx, created.ID)
	if len(entries) != 1 || string(entries[0].Value) != "300" {
		t.Fatalf("replay changed state: %v", entries)
	}
}

func TestLogRejectsUnknownField(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	created, _ := s.CreateTrip(ctx, doc("alps"))

	err := s.UpsertLog(ctx, created.ID, []LogEntry{
		{Day: 2, Field: "kilometres", Value: json.RawMessage(`1`), UpdatedAt: 1},
	})
	if err == nil {
		t.Fatal("expected an unknown-field error")
	}
	entries, _ := s.Log(ctx, created.ID)
	if len(entries) != 0 {
		t.Errorf("a rejected batch must not partially apply: %v", entries)
	}
}

func TestKitLastWriteWins(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	created, _ := s.CreateTrip(ctx, doc("alps"))

	_ = s.UpsertKit(ctx, created.ID, []KitEntry{{ItemID: "law-gloves", Checked: true, UpdatedAt: 2000}})
	_ = s.UpsertKit(ctx, created.ID, []KitEntry{{ItemID: "law-gloves", Checked: false, UpdatedAt: 1000}})

	state, _ := s.KitState(ctx, created.ID)
	if len(state) != 1 || !state[0].Checked {
		t.Fatalf("older untick won: %v", state)
	}
}

func TestClearingLogLeavesKitAlone(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	created, _ := s.CreateTrip(ctx, doc("alps"))

	_ = s.UpsertLog(ctx, created.ID, []LogEntry{{Day: 2, Field: "km", Value: json.RawMessage(`10`), UpdatedAt: 1}})
	_ = s.UpsertKit(ctx, created.ID, []KitEntry{{ItemID: "x", Checked: true, UpdatedAt: 1}})

	if err := s.ClearLog(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if entries, _ := s.Log(ctx, created.ID); len(entries) != 0 {
		t.Errorf("log not cleared: %v", entries)
	}
	if state, _ := s.KitState(ctx, created.ID); len(state) != 1 {
		t.Errorf("clearing the log took the checklist with it: %v", state)
	}
}

func TestStaleDaysFollowsStops(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	d := doc("alps")
	created, _ := s.CreateTrip(ctx, d)

	stale, err := s.StaleDays(ctx, created.ID, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].N != 2 {
		t.Fatalf("day 2 should be the only routable stale day: %v", stale)
	}

	// Cache it, and it stops being stale.
	err = s.PutRoute(ctx, created.ID, Route{
		Day: 2, Polyline: "abc", Km: 210, Hours: 4.5,
		Signature: SignatureOf(d.Days[1].Stops),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale, _ = s.StaleDays(ctx, created.ID, d); len(stale) != 0 {
		t.Fatalf("expected nothing stale, got %v", stale)
	}

	// Move a stop and it is stale again.
	d.Days[1].Stops[1].Lat = 43.9
	if stale, _ = s.StaleDays(ctx, created.ID, d); len(stale) != 1 {
		t.Fatalf("moving a stop should invalidate its route: %v", stale)
	}
}

func TestSignatureIgnoresInsignificantPrecision(t *testing.T) {
	a := []trip.Stop{{Name: "A", Lat: 43.50001, Lon: 6.00001}}
	b := []trip.Stop{{Name: "A", Lat: 43.500012, Lon: 6.000014}}
	if SignatureOf(a) != SignatureOf(b) {
		t.Error("a sub-metre nudge should not invalidate a good route")
	}

	c := []trip.Stop{{Name: "A", Lat: 43.51, Lon: 6.0}}
	if SignatureOf(a) == SignatureOf(c) {
		t.Error("a real move must invalidate the route")
	}
}

func TestDeleteCascades(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	created, _ := s.CreateTrip(ctx, doc("alps"))

	_ = s.UpsertLog(ctx, created.ID, []LogEntry{{Day: 2, Field: "km", Value: json.RawMessage(`10`), UpdatedAt: 1}})
	if err := s.DeleteTrip(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if entries, _ := s.Log(ctx, created.ID); len(entries) != 0 {
		t.Errorf("log outlived its trip: %v", entries)
	}
	if err := s.DeleteTrip(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete should be ErrNotFound, got %v", err)
	}
}
