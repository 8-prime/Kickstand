package trip

import (
	"fmt"
	"regexp"
	"strings"
)

// FieldError points at one problem, by path, so the caller can fix exactly
// that field. Paths look like `days[4].stops[2].lon`.
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Path + ": " + e.Message }

// ValidationError carries every problem found in one pass.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (v *ValidationError) Error() string {
	parts := make([]string, len(v.Errors))
	for i, e := range v.Errors {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Validate checks a whole document and returns every problem at once.
//
// One pass, all errors: the caller is often a language model that should get
// the complete list to fix rather than being walked through failures one at a
// time.
//
// Warnings are things that are probably wrong but legitimately might not be —
// they never block an import.
func Validate(t *Trip) (errs []FieldError, warnings []FieldError) {
	add := func(path, format string, args ...any) {
		errs = append(errs, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
	}
	warn := func(path, format string, args ...any) {
		warnings = append(warnings, FieldError{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	if t == nil {
		return []FieldError{{Path: "", Message: "the document is empty"}}, nil
	}

	// ---- identity -------------------------------------------------------
	if t.SchemaVersion == 0 {
		warn("schemaVersion", "missing; assuming %d", SchemaVersion)
	} else if t.SchemaVersion > SchemaVersion {
		add("schemaVersion", "this server understands up to version %d, got %d",
			SchemaVersion, t.SchemaVersion)
	}
	if strings.TrimSpace(t.Slug) == "" {
		add("slug", "required — a short lowercase name like \"provence\"")
	} else if !slugRe.MatchString(t.Slug) {
		add("slug", "%q must be lowercase letters, digits and single hyphens", t.Slug)
	}
	if strings.TrimSpace(t.Name) == "" {
		add("name", "required")
	}

	// ---- bounds ---------------------------------------------------------
	checkLat("bounds.lat0", t.Bounds.Lat0, add)
	checkLat("bounds.lat1", t.Bounds.Lat1, add)
	checkLon("bounds.lon0", t.Bounds.Lon0, add)
	checkLon("bounds.lon1", t.Bounds.Lon1, add)
	if t.Bounds.Lat0 >= t.Bounds.Lat1 {
		add("bounds.lat0", "must be south of bounds.lat1 (%g is not below %g)",
			t.Bounds.Lat0, t.Bounds.Lat1)
	}
	if t.Bounds.Lon0 >= t.Bounds.Lon1 {
		add("bounds.lon0", "must be west of bounds.lon1 (%g is not below %g)",
			t.Bounds.Lon0, t.Bounds.Lon1)
	}

	if t.VanIn < 0 {
		add("vanIn", "cannot be negative")
	}
	if t.VanOut < 0 {
		add("vanOut", "cannot be negative")
	}

	// ---- days -----------------------------------------------------------
	if len(t.Days) == 0 {
		add("days", "a trip needs at least one day")
	}
	seenDay := map[int]int{} // day number → first index that used it
	for i, d := range t.Days {
		p := fmt.Sprintf("days[%d]", i)

		if d.N <= 0 {
			add(p+".n", "day numbers start at 1, got %d", d.N)
		} else if first, dup := seenDay[d.N]; dup {
			add(p+".n", "day %d is already defined at days[%d]", d.N, first)
		} else {
			seenDay[d.N] = i
		}

		if strings.TrimSpace(d.Title) == "" {
			add(p+".title", "required")
		}
		switch d.Type {
		case Ride, Van:
		case "":
			add(p+".type", "required — %q or %q", Ride, Van)
		default:
			add(p+".type", "%q is not a day type; use %q or %q", d.Type, Ride, Van)
		}
		if d.Km < 0 {
			add(p+".km", "cannot be negative")
		}
		if d.Van < 0 {
			add(p+".van", "cannot be negative")
		}
		if d.Hours < 0 {
			add(p+".hours", "cannot be negative")
		}
		if d.Type == Ride && d.Km == 0 && len(d.Stops) > 0 {
			warn(p+".km", "a riding day with stops but no planned distance")
		}

		for j, s := range d.Stops {
			sp := fmt.Sprintf("%s.stops[%d]", p, j)
			if strings.TrimSpace(s.Name) == "" {
				add(sp+".name", "required")
			}
			checkLat(sp+".lat", s.Lat, add)
			checkLon(sp+".lon", s.Lon, add)
			// Outside the map window is usually a swapped lat/lon. Not fatal:
			// a day genuinely can run off the edge of the opening view.
			if outside(s.Lat, s.Lon, t.Bounds) {
				warn(sp, "%s at %g,%g falls outside bounds — check lat and lon are not swapped",
					s.Name, s.Lat, s.Lon)
			}
		}
		if len(d.Stops) == 1 {
			warn(p+".stops", "a single stop cannot be routed; give it two or none")
		}
	}

	// Day numbers should run 1..N with no holes, or the spine has gaps.
	if len(seenDay) > 0 {
		for n := 1; n <= len(t.Days); n++ {
			if _, ok := seenDay[n]; !ok {
				add("days", "day %d is missing — days must run 1 to %d", n, len(t.Days))
			}
		}
	}

	// ---- bases ----------------------------------------------------------
	seenBase := map[int]int{}
	for i, b := range t.Bases {
		p := fmt.Sprintf("bases[%d]", i)
		if b.Index <= 0 {
			add(p+".index", "base numbers start at 1, got %d", b.Index)
		} else if first, dup := seenBase[b.Index]; dup {
			add(p+".index", "base %d is already defined at bases[%d]", b.Index, first)
		} else {
			seenBase[b.Index] = i
		}
		if strings.TrimSpace(b.Name) == "" {
			add(p+".name", "required")
		}
		checkLat(p+".lat", b.Lat, add)
		checkLon(p+".lon", b.Lon, add)
		if b.ArriveDay <= 0 {
			add(p+".arriveDay", "required — the day you pitch here")
		} else if _, ok := seenDay[b.ArriveDay]; !ok {
			add(p+".arriveDay", "%d is beyond the %d days in this trip", b.ArriveDay, len(t.Days))
		}
	}

	// ---- campsites ------------------------------------------------------
	for i, c := range t.Campsites {
		p := fmt.Sprintf("campsites[%d]", i)
		switch c.Status {
		case Picked:
			if strings.TrimSpace(c.Name) == "" {
				add(p+".name", "required when status is %q", Picked)
			}
			if c.Rating < 0 || c.Rating > 5 {
				add(p+".rating", "%g is not a 0–5 rating", c.Rating)
			}
		case NotResearched:
		case "":
			add(p+".status", "required — %q or %q", Picked, NotResearched)
		default:
			add(p+".status", "%q is not a status; use %q or %q", c.Status, Picked, NotResearched)
		}
		if c.Base != 0 {
			if _, ok := seenBase[c.Base]; !ok && len(t.Bases) > 0 {
				add(p+".base", "no base %d in this trip", c.Base)
			}
		}
		if c.Lat != 0 || c.Lon != 0 {
			checkLat(p+".lat", c.Lat, add)
			checkLon(p+".lon", c.Lon, add)
		}
	}

	// ---- kit ------------------------------------------------------------
	seenItem := map[string]string{} // id → path that first used it
	for i, g := range t.Kit {
		p := fmt.Sprintf("kit[%d]", i)
		if strings.TrimSpace(g.Group) == "" {
			add(p+".group", "required")
		}
		for j, it := range g.Items {
			ip := fmt.Sprintf("%s.items[%d]", p, j)
			switch {
			case strings.TrimSpace(it.ID) == "":
				add(ip+".id", "required — tick state is keyed by it")
			default:
				if first, dup := seenItem[it.ID]; dup {
					add(ip+".id", "duplicate id %q, already used at %s", it.ID, first)
				} else {
					seenItem[it.ID] = ip
				}
			}
			if strings.TrimSpace(it.Title) == "" {
				add(ip+".title", "required")
			}
		}
	}

	return errs, warnings
}

// Normalize fills in the defaults an import is allowed to omit. Call it after
// Validate reports no errors.
func Normalize(t *Trip) {
	if t.SchemaVersion == 0 {
		t.SchemaVersion = SchemaVersion
	}
	// A campsite's base name is derivable; keep it in sync so the client
	// never has to join the two lists.
	byIndex := map[int]string{}
	for _, b := range t.Bases {
		byIndex[b.Index] = b.Name
	}
	for i := range t.Campsites {
		if name, ok := byIndex[t.Campsites[i].Base]; ok {
			t.Campsites[i].BaseName = name
		}
	}
}

func checkLat(path string, v float64, add func(string, string, ...any)) {
	if v < -90 || v > 90 {
		add(path, "%g is not a latitude (-90 to 90)", v)
	}
}

func checkLon(path string, v float64, add func(string, string, ...any)) {
	if v < -180 || v > 180 {
		add(path, "%g is not a longitude (-180 to 180)", v)
	}
}

func outside(lat, lon float64, b Bounds) bool {
	// A degree of slack: stops just off the edge of the opening window are
	// normal, a stop in another country is not.
	const slack = 1.0
	return lat < b.Lat0-slack || lat > b.Lat1+slack ||
		lon < b.Lon0-slack || lon > b.Lon1+slack
}
