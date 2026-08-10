// Package trip defines the trip document: the single JSON object that
// describes a whole trip, and the validation that keeps an imported one
// honest.
//
// The document is stored and served as-is. Nothing here is shredded into
// tables, because a trip is edited as a document — export is a read and
// import is a write.
package trip

// SchemaVersion is the current document version. Bump it only for a change
// that older documents cannot satisfy, and add a migration when you do.
const SchemaVersion = 1

// Trip is one complete plan.
type Trip struct {
	SchemaVersion int    `json:"schemaVersion"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Subtitle      string `json:"subtitle,omitempty"`
	Origin        string `json:"origin,omitempty"`
	Dates         string `json:"dates,omitempty"`

	Bounds Bounds `json:"bounds"`

	// Van kilometres out to the first base and home from the last.
	VanIn  float64 `json:"vanIn"`
	VanOut float64 `json:"vanOut"`

	Bases []Base `json:"bases"`
	Days  []Day  `json:"days"`

	Campsites         []Campsite `json:"campsites,omitempty"`
	RejectedCampsites []string   `json:"rejectedCampsites,omitempty"`
	CampsiteCaveat    string     `json:"campsiteCaveat,omitempty"`

	Kit []KitGroup `json:"kit,omitempty"`
}

// Bounds is the map window the trip opens at.
type Bounds struct {
	Lat0 float64 `json:"lat0"`
	Lat1 float64 `json:"lat1"`
	Lon0 float64 `json:"lon0"`
	Lon1 float64 `json:"lon1"`
}

// Base is a camp you sleep at for several nights and ride out from.
type Base struct {
	Index int     `json:"index"`
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	// The day you arrive and pitch.
	ArriveDay int    `json:"arriveDay"`
	Nights    string `json:"nights,omitempty"`
}

// DayType separates days you ride from days the van moves.
type DayType string

const (
	Ride DayType = "ride"
	Van  DayType = "van"
)

// Day is one numbered day of the trip.
type Day struct {
	N     int     `json:"n"`
	Date  string  `json:"date"`
	Type  DayType `json:"type"`
	Title string  `json:"title"`
	// Which base you sleep at, as written on the roadbook.
	Base   string `json:"base,omitempty"`
	Detail string `json:"detail,omitempty"`

	// Planned riding kilometres. Zero on a pure transfer day.
	Km float64 `json:"km"`
	// Van transfer kilometres. Zero on a riding day.
	Van float64 `json:"van"`
	// Planned saddle or driving time.
	Hours float64 `json:"hours"`

	Stops []Stop `json:"stops"`
}

// Stop is a place the day passes through, in order.
//
// Deliberately an object rather than a [name, lat, lon] triple: a positional
// triple invites a generator to swap lat and lon, and nothing catches that
// but a map that looks wrong.
type Stop struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// CampsiteStatus records whether a base has somewhere to sleep yet.
type CampsiteStatus string

const (
	Picked        CampsiteStatus = "picked"
	NotResearched CampsiteStatus = "not-researched"
)

// Campsite is where you pitch at a given base.
type Campsite struct {
	Base     int            `json:"base"`
	BaseName string         `json:"baseName,omitempty"`
	Status   CampsiteStatus `json:"status"`

	Name    string  `json:"name,omitempty"`
	Rating  float64 `json:"rating,omitempty"`
	Reviews int     `json:"reviews,omitempty"`
	Phone   string  `json:"phone,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	// True when lat/lon point at the base town rather than the pitch itself.
	CoordsApprox bool   `json:"coordsApprox,omitempty"`
	Note         string `json:"note,omitempty"`

	ClosingDateVerified bool `json:"closingDateVerified"`
}

// KitGroup is one section of the pre-departure checklist.
type KitGroup struct {
	Group string `json:"group"`
	// Legal groups carry a fine rather than an inconvenience, and are shown
	// differently.
	Legal bool      `json:"legal,omitempty"`
	Items []KitItem `json:"items"`
}

// KitItem is one thing to pack, do, or confirm.
type KitItem struct {
	// Stable across edits: tick state is keyed by it. Renaming an id loses
	// whatever was ticked against the old one.
	ID    string `json:"id"`
	Title string `json:"title"`
	Why   string `json:"why,omitempty"`
	// A fine, or a thing still to confirm: "€135", "CHECK", "NEEDED".
	Flag string `json:"flag,omitempty"`
}

// DayByNumber returns the day with the given number, if the trip has one.
func (t *Trip) DayByNumber(n int) (Day, bool) {
	for _, d := range t.Days {
		if d.N == n {
			return d, true
		}
	}
	return Day{}, false
}

// RoutableDays are the days with enough stops to ask a router about. The
// Bremen bookends have no stop list and are skipped.
func (t *Trip) RoutableDays() []Day {
	var out []Day
	for _, d := range t.Days {
		if len(d.Stops) >= 2 {
			out = append(out, d)
		}
	}
	return out
}
