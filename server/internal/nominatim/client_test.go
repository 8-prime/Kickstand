package nominatim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"bike-trip/server/internal/trip"
)

// fakeUpstream stands in for Nominatim and records what it was asked.
type fakeUpstream struct {
	mu       sync.Mutex
	requests []*url.URL
	agents   []string
	body     string
	status   int
}

func (f *fakeUpstream) server(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.URL)
		f.agents = append(f.agents, r.Header.Get("User-Agent"))
		f.mu.Unlock()

		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(f.body))
	}))
	t.Cleanup(srv.Close)

	c := New()
	c.BaseURL = srv.URL
	// The real interval is over a second, which no test should sit through.
	// What matters here is that a gap is enforced at all.
	c.MinInterval = 30 * time.Millisecond
	return c
}

func (f *fakeUpstream) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

const searchBody = `[
  {"lat":"48.0989","lon":"-4.2089","name":"Locronan","display_name":"Locronan, Finistère, France","addresstype":"village"},
  {"lat":"48.1000","lon":"-4.2000","name":"Locronan","display_name":"Locronan, Quebec","addresstype":"hamlet"}
]`

func TestSearchParsesQuotedCoordinates(t *testing.T) {
	up := &fakeUpstream{body: searchBody}
	c := up.server(t)

	places, err := c.Search(context.Background(), "locronan", nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(places) != 2 {
		t.Fatalf("got %d places, want 2", len(places))
	}
	// Nominatim quotes its coordinates; a client that treated them as numbers
	// would silently get zeros and put every stop off West Africa.
	if places[0].Lat != 48.0989 || places[0].Lon != -4.2089 {
		t.Errorf("first place at %g,%g, want 48.0989,-4.2089", places[0].Lat, places[0].Lon)
	}
	if places[0].Name != "Locronan" || places[0].Kind != "village" {
		t.Errorf("first place = %+v", places[0])
	}
	// The full address is the only thing separating the two.
	if places[0].DisplayName == places[1].DisplayName {
		t.Error("results must carry their full addresses to be distinguishable")
	}
}

func TestSearchIdentifiesItselfAndBiasesToTheTrip(t *testing.T) {
	up := &fakeUpstream{body: searchBody}
	c := up.server(t)

	bounds := &trip.Bounds{Lat0: 47.5, Lat1: 48.9, Lon0: -5.2, Lon1: -1.8}
	if _, err := c.Search(context.Background(), "locronan", bounds); err != nil {
		t.Fatalf("search: %v", err)
	}

	up.mu.Lock()
	defer up.mu.Unlock()

	// Required by the OSM usage policy, and the reason this call is not made
	// from the browser at all.
	if up.agents[0] == "" || up.agents[0] == "Go-http-client/1.1" {
		t.Errorf("User-Agent = %q, want an identifying one", up.agents[0])
	}

	q := up.requests[0].Query()
	if got := q.Get("viewbox"); got != "-5.2,48.9,-1.8,47.5" {
		t.Errorf("viewbox = %q, want left,top,right,bottom in lon,lat order", got)
	}
	// A bias, not a filter: a stop just off the edge of the map window must
	// still be findable.
	if q.Get("bounded") != "0" {
		t.Errorf("bounded = %q, want 0", q.Get("bounded"))
	}
}

func TestRepeatedSearchIsServedFromCache(t *testing.T) {
	up := &fakeUpstream{body: searchBody}
	c := up.server(t)

	for i := 0; i < 3; i++ {
		if _, err := c.Search(context.Background(), "locronan", nil); err != nil {
			t.Fatalf("search %d: %v", i, err)
		}
	}
	if up.count() != 1 {
		t.Errorf("upstream saw %d requests for the same query, want 1", up.count())
	}
	if c.Requests() != 1 {
		t.Errorf("client counted %d requests, want 1", c.Requests())
	}

	// A different query is a different question.
	if _, err := c.Search(context.Background(), "quimper", nil); err != nil {
		t.Fatalf("second query: %v", err)
	}
	if up.count() != 2 {
		t.Errorf("upstream saw %d requests, want 2", up.count())
	}
}

func TestRequestsAreSpacedOut(t *testing.T) {
	up := &fakeUpstream{body: searchBody}
	c := up.server(t)

	start := time.Now()
	for _, q := range []string{"one", "two", "three"} {
		if _, err := c.Search(context.Background(), q, nil); err != nil {
			t.Fatalf("search %s: %v", q, err)
		}
	}
	// Three distinct queries, two gaps. The OSM policy is a hard limit, not a
	// suggestion, so the pacing has to be in the client rather than left to
	// however fast someone types.
	if elapsed := time.Since(start); elapsed < 2*c.MinInterval {
		t.Errorf("three requests took %v, want at least %v", elapsed, 2*c.MinInterval)
	}
}

func TestShortQueriesNeverReachUpstream(t *testing.T) {
	up := &fakeUpstream{body: searchBody}
	c := up.server(t)

	for _, q := range []string{"", " ", "l"} {
		places, err := c.Search(context.Background(), q, nil)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		if len(places) != 0 {
			t.Errorf("search %q returned %d places", q, len(places))
		}
	}
	if up.count() != 0 {
		t.Errorf("upstream saw %d requests for queries too short to mean anything", up.count())
	}
}

const reverseBody = `{
  "lat":"48.1030","lon":"-4.2110","name":"",
  "display_name":"Plonévez-Porzay, Finistère, France",
  "addresstype":"village",
  "address":{"village":"Plonévez-Porzay","county":"Finistère"}
}`

func TestReverseNamesAPointWithoutMovingIt(t *testing.T) {
	up := &fakeUpstream{body: reverseBody}
	c := up.server(t)

	place, err := c.Reverse(context.Background(), 48.10305, -4.21107)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	// Nominatim often answers with no `name` at all; falling through to the
	// settlement is what keeps a dropped stop from being called "".
	if place.Name != "Plonévez-Porzay" {
		t.Errorf("name = %q, want the village", place.Name)
	}
	// The answer is about the point that was asked for. Snapping the stop to
	// the centre of the village would move a pin the rider just placed.
	if place.Lat != 48.10305 || place.Lon != -4.21107 {
		t.Errorf("reverse moved the point to %g,%g", place.Lat, place.Lon)
	}
}

func TestUpstreamFailuresAreSaidPlainly(t *testing.T) {
	up := &fakeUpstream{body: `{}`, status: http.StatusTooManyRequests}
	c := up.server(t)

	_, err := c.Search(context.Background(), "locronan", nil)
	if err == nil {
		t.Fatal("a 429 should be an error")
	}
	var pe *Error
	if !asError(err, &pe) {
		t.Fatalf("error is %T, want a *nominatim.Error the interface can show", err)
	}
	if pe.Message == "" {
		t.Error("the message is what gets shown to a person; it cannot be empty")
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}
