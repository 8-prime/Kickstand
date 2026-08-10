package trip

import (
	"encoding/json"
	"strings"
	"testing"
)

const patchDoc = `{
  "slug": "alps",
  "name": "Alps",
  "days": [
    { "n": 1, "title": "Out", "km": 0, "stops": [] },
    { "n": 2, "title": "Loop", "km": 200,
      "stops": [{ "name": "A", "lat": 43.5, "lon": 6.0 }] }
  ],
  "campsites": [{ "base": 1, "status": "picked", "name": "Camping", "phone": "+33 1" }],
  "kit": [{ "group": "Docs", "items": [{ "id": "a", "title": "Passport" }] }]
}`

func apply(t *testing.T, ops ...Op) map[string]any {
	t.Helper()
	out, errs := ApplyPatch([]byte(patchDoc), ops)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func op(path, value string) Op { return Op{Path: path, Value: json.RawMessage(value)} }

func TestPatchSetsScalars(t *testing.T) {
	got := apply(t,
		op("name", `"Alpes Maritimes"`),
		op("days[1].km", `230`),
		op("campsites[0].phone", `"+33 4 93 04 10 48"`),
		op("kit[0].items[0].title", `"Passport or ID"`),
	)

	if got["name"] != "Alpes Maritimes" {
		t.Errorf("name: %v", got["name"])
	}
	days := got["days"].([]any)
	if days[1].(map[string]any)["km"] != float64(230) {
		t.Errorf("days[1].km: %v", days[1])
	}
	camps := got["campsites"].([]any)
	if camps[0].(map[string]any)["phone"] != "+33 4 93 04 10 48" {
		t.Errorf("phone: %v", camps[0])
	}
	kit := got["kit"].([]any)[0].(map[string]any)["items"].([]any)
	if kit[0].(map[string]any)["title"] != "Passport or ID" {
		t.Errorf("kit title: %v", kit[0])
	}
}

func TestPatchLeavesEverythingElseAlone(t *testing.T) {
	got := apply(t, op("days[1].km", `230`))

	days := got["days"].([]any)
	if days[0].(map[string]any)["title"] != "Out" {
		t.Error("day 1 was disturbed")
	}
	stops := days[1].(map[string]any)["stops"].([]any)
	if stops[0].(map[string]any)["name"] != "A" {
		t.Error("stops were disturbed")
	}
}

// Adding and removing list items is done by setting the whole list.
func TestPatchReplacesWholeList(t *testing.T) {
	got := apply(t, op("days[1].stops",
		`[{"name":"A","lat":43.5,"lon":6.0},{"name":"B","lat":43.6,"lon":6.2}]`))

	stops := got["days"].([]any)[1].(map[string]any)["stops"].([]any)
	if len(stops) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(stops))
	}
}

func TestPatchReportsBadPaths(t *testing.T) {
	cases := []struct {
		name, path, want string
	}{
		{"index past end", "days[9].km", "past the end"},
		{"unknown field", "days[1].nosuch.deeper", "no field"},
		{"index into object", "name[0]", "not a list"},
		{"field on a number", "days[1].km.value", "not an object"},
		{"empty", "", "empty path"},
		{"unclosed bracket", "days[1.km", "unclosed ["},
		{"non-numeric index", "days[first].km", "not a list index"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := ApplyPatch([]byte(patchDoc), []Op{op(tc.path, `1`)})
			if len(errs) != 1 {
				t.Fatalf("expected one error, got %v", errs)
			}
			if !strings.Contains(errs[0].Message, tc.want) {
				t.Errorf("message %q does not mention %q", errs[0].Message, tc.want)
			}
			if errs[0].Path != tc.path {
				t.Errorf("error path %q, want %q", errs[0].Path, tc.path)
			}
		})
	}
}

// A failing op must not leave half a patch applied.
func TestPatchIsAllOrNothing(t *testing.T) {
	out, errs := ApplyPatch([]byte(patchDoc), []Op{
		op("name", `"Changed"`),
		op("days[9].km", `1`),
	})
	if out != nil {
		t.Error("a failed patch must not return a document")
	}
	if len(errs) != 1 || errs[0].Path != "days[9].km" {
		t.Fatalf("expected the bad op reported, got %v", errs)
	}
}

func TestPatchCollectsEveryBadOp(t *testing.T) {
	_, errs := ApplyPatch([]byte(patchDoc), []Op{
		op("days[9].km", `1`),
		op("nosuch.field", `1`),
	})
	if len(errs) != 2 {
		t.Fatalf("expected both ops reported, got %v", errs)
	}
}
