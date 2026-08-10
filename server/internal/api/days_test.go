package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"bike-trip/server/internal/nominatim"
	"bike-trip/server/internal/osrm"
	"bike-trip/server/internal/store"
)

type dayShape struct {
	N     int    `json:"n"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Stops []any  `json:"stops"`
}

type docShape struct {
	Days  []dayShape `json:"days"`
	Bases []struct {
		Index     int `json:"index"`
		ArriveDay int `json:"arriveDay"`
	} `json:"bases"`
}

type dayResponse struct {
	Revision int             `json:"revision"`
	Doc      json.RawMessage `json:"doc"`
	Log      []struct {
		Day   int             `json:"day"`
		Field string          `json:"field"`
		Value json.RawMessage `json:"value"`
	} `json:"log"`
	Warnings []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"warnings"`
}

func (r dayResponse) days(t *testing.T) []dayShape {
	t.Helper()
	var d docShape
	if err := json.Unmarshal(r.Doc, &d); err != nil {
		t.Fatalf("decode doc: %v", err)
	}
	return d.Days
}

func (r dayResponse) km(field int) (float64, bool) {
	for _, e := range r.Log {
		if e.Day == field && e.Field == "km" {
			var v float64
			if json.Unmarshal(e.Value, &v) == nil {
				return v, true
			}
		}
	}
	return 0, false
}

func logKm(t *testing.T, h http.Handler, token string, day int, km float64) {
	t.Helper()
	rec := do(t, h, call{
		method: "PUT",
		path:   "/api/trips/" + token + "/log",
		body: map[string]any{"entries": []any{
			map[string]any{"day": day, "field": "km", "value": km, "updatedAt": 1},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("log day %d: %d %s", day, rec.Code, rec.Body)
	}
}

func TestInsertDayRenumbersAndKeepsTheLogWithItsDay(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	// The sample trip is two days. Log against the second, then push a new day
	// in front of it: the reading has to arrive with day 2, not stay on the
	// number that another day now holds.
	logKm(t, h, tr.EditToken, 2, 214)

	rec := do(t, h, call{
		method: "POST",
		path:   "/api/trips/" + tr.EditToken + "/days",
		body:   map[string]any{"op": "insert", "after": 1},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("insert: %d %s", rec.Code, rec.Body)
	}

	got := decode[dayResponse](t, rec)
	days := got.days(t)
	if len(days) != 3 {
		t.Fatalf("got %d days, want 3", len(days))
	}
	for i, d := range days {
		if d.N != i+1 {
			t.Errorf("days[%d].n = %d, want %d — numbers must run 1..N", i, d.N, i+1)
		}
	}
	if days[1].Title != "New day" {
		t.Errorf("days[1] = %q, want the inserted one", days[1].Title)
	}
	// A new day must carry an empty stop list, never null: the client maps
	// over it without checking.
	if days[1].Stops == nil {
		t.Error("the inserted day has a null stop list")
	}

	if km, ok := got.km(3); !ok || km != 214 {
		t.Errorf("logged km ended up on day 3 = (%g, %v), want 214", km, ok)
	}
	if _, taken := got.km(2); taken {
		t.Error("the inserted day inherited the previous day's odometer reading")
	}
}

func TestDeleteDayTakesItsLogWithIt(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	logKm(t, h, tr.EditToken, 1, 900)
	logKm(t, h, tr.EditToken, 2, 214)

	rec := do(t, h, call{
		method: "POST",
		path:   "/api/trips/" + tr.EditToken + "/days",
		body:   map[string]any{"op": "delete", "day": 1},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}

	got := decode[dayResponse](t, rec)
	if days := got.days(t); len(days) != 1 || days[0].N != 1 || days[0].Title != "Loop" {
		t.Fatalf("days = %+v, want the second day renumbered to 1", days)
	}
	if km, ok := got.km(1); !ok || km != 214 {
		t.Errorf("day 1 logged (%g, %v), want the surviving day's 214", km, ok)
	}
	if len(got.Log) != 1 {
		t.Errorf("log has %d entries, want the deleted day's gone: %+v", len(got.Log), got.Log)
	}

	// The base pitched on the day that was removed, so it had to be moved and
	// the rider told about it.
	if len(got.Warnings) == 0 {
		t.Error("moving a base's arrival day silently is worse than saying so")
	}
}

func TestMoveDayReorders(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	logKm(t, h, tr.EditToken, 2, 214)

	rec := do(t, h, call{
		method: "POST",
		path:   "/api/trips/" + tr.EditToken + "/days",
		body:   map[string]any{"op": "move", "day": 2, "to": 1},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}

	got := decode[dayResponse](t, rec)
	days := got.days(t)
	if days[0].Title != "Loop" || days[1].Title != "Out" {
		t.Fatalf("days = %+v, want Loop first", days)
	}
	if km, ok := got.km(1); !ok || km != 214 {
		t.Errorf("day 1 logged (%g, %v), want the moved day's 214", km, ok)
	}
}

func TestViewTokenCannotChangeDays(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method: "POST",
		path:   "/api/trips/" + tr.ViewToken + "/days",
		body:   map[string]any{"op": "delete", "day": 1},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403 for a read-only link", rec.Code)
	}
}

func TestStaleDayEditIsRejected(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method:  "POST",
		path:    "/api/trips/" + tr.EditToken + "/days",
		body:    map[string]any{"op": "insert", "after": 1},
		headers: map[string]string{"If-Match": "99"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409 for a stale revision", rec.Code)
	}
}

func TestNonsenseDayOpsAreRefused(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	for _, body := range []map[string]any{
		{"op": "delete", "day": 99},
		{"op": "move", "day": 99, "to": 1},
		{"op": "shuffle"},
	} {
		rec := do(t, h, call{
			method: "POST",
			path:   "/api/trips/" + tr.EditToken + "/days",
			body:   body,
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%v: got %d, want 400", body, rec.Code)
		}
	}
}

func TestPlaceLookupNeedsAnEditLink(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	// Each lookup spends an upstream request against a service running on
	// goodwill; a read-only link has nothing to place and cannot spend them.
	for _, path := range []string{
		"/api/trips/" + tr.ViewToken + "/places?q=locronan",
		"/api/trips/" + tr.ViewToken + "/places/reverse?lat=48&lon=-4",
	} {
		rec := do(t, h, call{method: "GET", path: path})
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s: got %d, want 403", path, rec.Code)
		}
	}
}

// geocodingServer is a server whose place lookups reach a stub instead of the
// public OSM geocoder.
func geocodingServer(t *testing.T, body string) http.Handler {
	t.Helper()

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(up.Close)

	st, err := store.Open(filepath.Join(t.TempDir(), "places.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	geo := nominatim.New()
	geo.BaseURL = up.URL
	geo.MinInterval = time.Millisecond

	return New(Options{Store: st, OSRM: osrm.New(), Nominatim: geo, AdminToken: adminToken})
}

func TestSearchReturnsPlacesBiasedToTheTrip(t *testing.T) {
	h := geocodingServer(t, `[
	  {"lat":"43.55","lon":"6.10","name":"Bargème","display_name":"Bargème, Var, France","addresstype":"village"}
	]`)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method: "GET",
		path:   "/api/trips/" + tr.EditToken + "/places?q=bargeme",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("search: %d %s", rec.Code, rec.Body)
	}

	got := decode[struct {
		Places []struct {
			Name        string  `json:"name"`
			DisplayName string  `json:"displayName"`
			Lat         float64 `json:"lat"`
			Lon         float64 `json:"lon"`
		} `json:"places"`
	}](t, rec)

	if len(got.Places) != 1 {
		t.Fatalf("got %d places, want 1: %s", len(got.Places), rec.Body)
	}
	p := got.Places[0]
	if p.Name != "Bargème" || p.Lat != 43.55 || p.Lon != 6.10 {
		t.Errorf("place = %+v", p)
	}
	if p.DisplayName == "" {
		t.Error("the full address is what tells two same-named places apart")
	}
}

func TestReverseLookupRejectsNonsenseCoordinates(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	for _, q := range []string{"", "?lat=48", "?lat=abc&lon=-4", "?lat=200&lon=-4"} {
		rec := do(t, h, call{
			method: "GET",
			path:   "/api/trips/" + tr.EditToken + "/places/reverse" + q,
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("reverse %q: got %d, want 400", q, rec.Code)
		}
	}
}
