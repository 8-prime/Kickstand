package trip

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The seeded documents are the reference: if these stop validating, the
// schema and the data have drifted apart.
func TestSeedsValidate(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "seed", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no seed files found: %v", err)
	}

	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			var doc Trip
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			errs, warns := Validate(&doc)
			for _, e := range errs {
				t.Errorf("unexpected error %s: %s", e.Path, e.Message)
			}
			for _, w := range warns {
				t.Logf("warning %s: %s", w.Path, w.Message)
			}
		})
	}
}

// A minimal document that passes, used as the base for the failure cases so
// each test changes exactly one thing.
func good() *Trip {
	return &Trip{
		SchemaVersion: 1,
		Slug:          "test-trip",
		Name:          "Test",
		Bounds:        Bounds{Lat0: 43, Lat1: 44, Lon0: 5, Lon1: 7},
		Bases: []Base{
			{Index: 1, Name: "Base One", Lat: 43.5, Lon: 6, ArriveDay: 1},
		},
		Days: []Day{
			{N: 1, Type: Van, Title: "Out", Van: 900},
			{N: 2, Type: Ride, Title: "Loop", Km: 200, Hours: 5, Stops: []Stop{
				{Name: "A", Lat: 43.5, Lon: 6},
				{Name: "B", Lat: 43.6, Lon: 6.2},
			}},
		},
		Kit: []KitGroup{
			{Group: "Documents", Items: []KitItem{{ID: "doc-id", Title: "Passport"}}},
		},
	}
}

func TestGoodDocumentPasses(t *testing.T) {
	errs, _ := Validate(good())
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateReportsEveryProblemAtOnce(t *testing.T) {
	doc := good()
	// Four independent mistakes, the kind an import actually makes.
	doc.Days[1].Stops[0].Lat = 6.0 // swapped with lon
	doc.Days[1].Stops[0].Lon = 43.5
	doc.Days[1].Title = ""                                                             // missing required field
	doc.Bases[0].ArriveDay = 12                                                        // beyond the trip
	doc.Kit[0].Items = append(doc.Kit[0].Items, KitItem{ID: "doc-id", Title: "Again"}) // duplicate id

	errs, warns := Validate(doc)

	want := map[string]bool{
		"days[1].title":      false,
		"bases[0].arriveDay": false,
		"kit[0].items[1].id": false,
	}
	for _, e := range errs {
		if _, ok := want[e.Path]; ok {
			want[e.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("no error reported for %s; got %v", path, errs)
		}
	}
	if len(errs) != len(want) {
		t.Errorf("expected exactly %d errors, got %d: %v", len(want), len(errs), errs)
	}

	// A swapped lat/lon is inside the legal ranges, so it lands as a warning
	// pointing at the likely cause rather than a hard failure.
	var swapWarned bool
	for _, w := range warns {
		if w.Path == "days[1].stops[0]" {
			swapWarned = true
		}
	}
	if !swapWarned {
		t.Errorf("expected a bounds warning for the swapped coordinate; got %v", warns)
	}
}

func TestImpossibleCoordinatesAreErrors(t *testing.T) {
	doc := good()
	doc.Days[1].Stops[1].Lat = 143.5

	errs, _ := Validate(doc)
	if len(errs) != 1 || errs[0].Path != "days[1].stops[1].lat" {
		t.Fatalf("expected one latitude error, got %v", errs)
	}
}

func TestDayNumbersMustBeContiguous(t *testing.T) {
	doc := good()
	doc.Days[1].N = 3 // leaves a hole at 2

	errs, _ := Validate(doc)
	if len(errs) != 1 || errs[0].Path != "days" {
		t.Fatalf("expected one gap error on days, got %v", errs)
	}
}

func TestUnknownDayTypeRejected(t *testing.T) {
	doc := good()
	doc.Days[1].Type = "cycling"

	errs, _ := Validate(doc)
	if len(errs) != 1 || errs[0].Path != "days[1].type" {
		t.Fatalf("expected one type error, got %v", errs)
	}
}

func TestPickedCampsiteNeedsAName(t *testing.T) {
	doc := good()
	doc.Campsites = []Campsite{{Base: 1, Status: Picked}}

	errs, _ := Validate(doc)
	if len(errs) != 1 || errs[0].Path != "campsites[0].name" {
		t.Fatalf("expected one campsite name error, got %v", errs)
	}
}

func TestNormalizeFillsBaseNames(t *testing.T) {
	doc := good()
	doc.SchemaVersion = 0
	doc.Campsites = []Campsite{{Base: 1, Status: NotResearched}}

	Normalize(doc)

	if doc.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion not defaulted: %d", doc.SchemaVersion)
	}
	if doc.Campsites[0].BaseName != "Base One" {
		t.Errorf("baseName not derived: %q", doc.Campsites[0].BaseName)
	}
}
