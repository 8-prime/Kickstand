// Package nominatim turns a place name into coordinates, and coordinates back
// into a place name.
//
// It lives on the server for the same reasons the router does — one polite
// client instead of one per rider — plus one that is specific to this service:
// the OSM usage policy requires an identifying User-Agent, and a browser
// cannot set that header. Calling Nominatim from the frontend would be both
// anonymous and against the terms.
package nominatim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bike-trip/server/internal/trip"
)

// DefaultBaseURL is the public OSM geocoder: no key, no account, and a usage
// policy rather than a quota. Point BaseURL elsewhere if you run your own.
const DefaultBaseURL = "https://nominatim.openstreetmap.org"

// Place is one candidate location, in the shape a Stop wants.
type Place struct {
	// Short label, which becomes the stop name: "Locronan".
	Name string `json:"name"`
	// The full address, so two places of the same name are distinguishable.
	DisplayName string `json:"displayName"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	// What kind of thing this is: "village", "peak", "viewpoint".
	Kind string `json:"kind,omitempty"`
}

// Client is a rate-limited Nominatim caller.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	// MinInterval is the shortest gap between two requests. The OSM policy is
	// an absolute maximum of one request per second — stricter than the
	// router's, and not a suggestion — so this is deliberately over a second.
	MinInterval time.Duration

	mu    sync.Mutex
	last  time.Time
	cache map[string][]Place

	requests atomic.Int64
}

// New returns a client with defaults that satisfy the OSM usage policy.
func New() *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
		MinInterval: 1100 * time.Millisecond,
		cache:       map[string][]Place{},
	}
}

// Requests reports how many calls have been made upstream. Tests assert on it
// to prove the cache is doing its job.
func (c *Client) Requests() int64 { return c.requests.Load() }

// Error is a lookup failure worth showing to a person.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// Search finds places matching a query, biased toward the trip's region.
//
// The bias is a nudge, not a filter: a trip's stops legitimately run off the
// edge of its opening map window, so a match outside the box still comes back,
// just lower down.
func (c *Client) Search(ctx context.Context, q string, near *trip.Bounds) ([]Place, error) {
	q = strings.TrimSpace(q)
	if len(q) < 2 {
		return []Place{}, nil
	}

	params := url.Values{}
	params.Set("q", q)
	params.Set("format", "jsonv2")
	params.Set("limit", "8")
	if near != nil {
		// left,top,right,bottom — the one place this package writes a
		// coordinate pair in lon,lat order.
		params.Set("viewbox", fmt.Sprintf("%g,%g,%g,%g",
			near.Lon0, near.Lat1, near.Lon1, near.Lat0))
		params.Set("bounded", "0")
	}

	key := params.Encode()
	if hit, ok := c.cached(key); ok {
		return hit, nil
	}

	var body []searchResult
	if err := c.get(ctx, "/search?"+key, &body); err != nil {
		return nil, err
	}

	places := make([]Place, 0, len(body))
	for _, r := range body {
		p, ok := r.place()
		if ok {
			places = append(places, p)
		}
	}

	c.store(key, places)
	return places, nil
}

// Reverse names the place at a coordinate, for a stop dropped on the map.
func (c *Client) Reverse(ctx context.Context, lat, lon float64) (Place, error) {
	params := url.Values{}
	params.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	params.Set("lon", strconv.FormatFloat(lon, 'f', -1, 64))
	params.Set("format", "jsonv2")
	// Village and suburb granularity: the right altitude to name a waypoint.
	// Higher zoom returns the house you happened to land on.
	params.Set("zoom", "14")
	params.Set("addressdetails", "1")

	key := "reverse?" + params.Encode()
	if hit, ok := c.cached(key); ok && len(hit) > 0 {
		return hit[0], nil
	}

	var body searchResult
	if err := c.get(ctx, "/reverse?"+params.Encode(), &body); err != nil {
		return Place{}, err
	}

	p, ok := body.place()
	if !ok {
		return Place{}, &Error{"nothing is mapped at that point"}
	}
	// A reverse lookup answers about the coordinate that was asked for, not the
	// centre of whatever village owns it — the pin must not jump on drop.
	p.Lat, p.Lon = lat, lon

	c.store(key, []Place{p})
	return p, nil
}

/* ------------------------------- plumbing -------------------------------- */

// searchResult covers both endpoints: /reverse returns one of these, /search
// returns a list of them.
type searchResult struct {
	// Strings, not numbers: Nominatim quotes its coordinates.
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	AddressType string `json:"addresstype"`
	Type        string `json:"type"`
	Address     struct {
		Village      string `json:"village"`
		Town         string `json:"town"`
		City         string `json:"city"`
		Municipality string `json:"municipality"`
		County       string `json:"county"`
	} `json:"address"`
}

func (r searchResult) place() (Place, bool) {
	lat, err := strconv.ParseFloat(r.Lat, 64)
	if err != nil {
		return Place{}, false
	}
	lon, err := strconv.ParseFloat(r.Lon, 64)
	if err != nil {
		return Place{}, false
	}

	kind := r.AddressType
	if kind == "" {
		kind = r.Type
	}

	return Place{
		Name:        r.label(),
		DisplayName: r.DisplayName,
		Lat:         lat,
		Lon:         lon,
		Kind:        kind,
	}, true
}

// label picks the shortest honest name for a place. Reverse lookups often come
// back with no `name` at all, so fall through the settlement fields and only
// then to the first component of the full address.
func (r searchResult) label() string {
	for _, s := range []string{
		r.Name,
		r.Address.Village, r.Address.Town, r.Address.City,
		r.Address.Municipality, r.Address.County,
	} {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	head, _, _ := strings.Cut(r.DisplayName, ",")
	return strings.TrimSpace(head)
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	if err := c.wait(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	// Required by the OSM usage policy: an anonymous client gets blocked, and
	// deserves to be.
	req.Header.Set("User-Agent", "bike-trip/1.0 (self-hosted trip planner)")

	c.requests.Add(1)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return &Error{"the place lookup service is unreachable"}
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusTooManyRequests {
		return &Error{"the place lookup service is rate limiting us; try again in a minute"}
	}
	if res.StatusCode != http.StatusOK {
		return &Error{fmt.Sprintf("the place lookup service returned %d", res.StatusCode)}
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		return &Error{"the place lookup service sent something unreadable"}
	}
	return nil
}

// wait spaces requests out so typing into a search box never bursts.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if gap := time.Until(c.last.Add(c.MinInterval)); gap > 0 {
		select {
		case <-time.After(gap):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.last = time.Now()
	return nil
}

func (c *Client) cached(key string) ([]Place, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hit, ok := c.cache[key]
	return hit, ok
}

func (c *Client) store(key string, places []Place) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = map[string][]Place{}
	}
	// Bounded by dropping the lot rather than tracking ages: a search cache is
	// pure latency relief, and a rebuilt one costs a few requests at worst.
	if len(c.cache) > 512 {
		c.cache = map[string][]Place{}
	}
	c.cache[key] = places
}
