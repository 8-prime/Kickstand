// Package osrm asks a routing service for the road geometry between a day's
// stops.
//
// This lives on the server rather than in the browser so that one fetch
// serves everyone on a trip, the public demo server sees a single polite
// client instead of one per rider, and a phone with no signal still gets
// geometry from the cached trip payload.
package osrm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bike-trip/server/internal/trip"
)

// DefaultBaseURL is the public OSRM demo server: no key, no account, and no
// service guarantee. Point BaseURL elsewhere if you run your own.
const DefaultBaseURL = "https://router.project-osrm.org/route/v1/driving"

// Result is one routed day.
type Result struct {
	// Encoded polyline, precision 5.
	Polyline string
	// Road distance in kilometres.
	Km float64
	// Driving time in hours, for a car.
	Hours float64
}

// Client is a rate-limited OSRM caller.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	// MinInterval is the shortest gap between two requests. The demo server
	// is a courtesy; a batch walks it rather than hammering it.
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time

	requests atomic.Int64
}

// New returns a client with sensible defaults for the public demo server.
func New() *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		HTTP:        &http.Client{Timeout: 20 * time.Second},
		MinInterval: 300 * time.Millisecond,
	}
}

// Requests reports how many calls have been made upstream. Tests assert on it
// to prove the cache is doing its job.
func (c *Client) Requests() int64 { return c.requests.Load() }

// Error is a routing failure worth showing to a person.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// Route returns the road geometry through the given stops, in order.
func (c *Client) Route(ctx context.Context, stops []trip.Stop) (Result, error) {
	if len(stops) < 2 {
		return Result{}, &Error{"a route needs at least two stops"}
	}

	// OSRM takes lon,lat. Every other coordinate in this project is lat,lon —
	// this is the one place that flips, and it stays here.
	var coords strings.Builder
	for i, s := range stops {
		if i > 0 {
			coords.WriteByte(';')
		}
		coords.WriteString(strconv.FormatFloat(s.Lon, 'f', -1, 64))
		coords.WriteByte(',')
		coords.WriteString(strconv.FormatFloat(s.Lat, 'f', -1, 64))
	}

	url := fmt.Sprintf("%s/%s?overview=simplified&geometries=polyline&alternatives=false&steps=false",
		c.BaseURL, coords.String())

	if err := c.wait(ctx); err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "bike-trip/1.0 (self-hosted trip planner)")

	c.requests.Add(1)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, &Error{"the routing service is unreachable"}
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusTooManyRequests {
		return Result{}, &Error{"the routing service is rate limiting us; try again in a minute"}
	}
	if res.StatusCode != http.StatusOK {
		return Result{}, &Error{fmt.Sprintf("the routing service returned %d", res.StatusCode)}
	}

	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Routes  []struct {
			Geometry string  `json:"geometry"`
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return Result{}, &Error{"the routing service sent something unreadable"}
	}
	if body.Code != "Ok" || len(body.Routes) == 0 {
		msg := body.Message
		if msg == "" {
			msg = fmt.Sprintf("no route found (%s)", body.Code)
		}
		return Result{}, &Error{msg}
	}

	r := body.Routes[0]
	return Result{
		Polyline: r.Geometry,
		Km:       round(r.Distance/1000, 0),
		Hours:    round(r.Duration/3600, 1),
	}, nil
}

// wait spaces requests out so a batch never bursts.
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

func round(v float64, places int) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'f', places, 64), 64)
	return f
}
