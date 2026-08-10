package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"bike-trip/server/internal/osrm"
	"bike-trip/server/internal/store"
	"bike-trip/server/internal/trip"
)

const adminToken = "test-admin-token"

func testServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	h := New(Options{Store: st, OSRM: osrm.New(), AdminToken: adminToken})
	return h, st
}

type call struct {
	method, path string
	body         any
	admin        bool
	headers      map[string]string
}

func do(t *testing.T, h http.Handler, c call) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if c.body != nil {
		raw, err := json.Marshal(c.body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(c.method, c.path, reader)
	if c.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.admin {
		req.Header.Set("X-Admin-Token", adminToken)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

func sampleDoc(slug string) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"slug":          slug,
		"name":          "Test " + slug,
		"bounds":        map[string]any{"lat0": 43, "lat1": 44, "lon0": 5, "lon1": 7},
		"vanIn":         900,
		"vanOut":        800,
		"bases": []any{
			map[string]any{"index": 1, "name": "Base", "lat": 43.5, "lon": 6, "arriveDay": 1},
		},
		"days": []any{
			map[string]any{"n": 1, "type": "van", "title": "Out", "km": 0, "van": 900, "hours": 0, "stops": []any{}},
			map[string]any{"n": 2, "type": "ride", "title": "Loop", "km": 200, "van": 0, "hours": 5,
				"stops": []any{
					map[string]any{"name": "A", "lat": 43.5, "lon": 6.0},
					map[string]any{"name": "B", "lat": 43.6, "lon": 6.2},
				}},
		},
		"kit": []any{
			map[string]any{"group": "Docs", "items": []any{
				map[string]any{"id": "doc-passport", "title": "Passport"},
			}},
		},
	}
}

type created struct {
	ID        string `json:"id"`
	Revision  int    `json:"revision"`
	Access    string `json:"access"`
	ViewToken string `json:"viewToken"`
	EditToken string `json:"editToken"`
}

func createTrip(t *testing.T, h http.Handler, slug string) created {
	t.Helper()
	rec := do(t, h, call{method: "POST", path: "/api/trips", body: sampleDoc(slug), admin: true})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	return decode[created](t, rec)
}

func TestAdminRoutesNeedTheAdminToken(t *testing.T) {
	h, _ := testServer(t)

	for _, c := range []call{
		{method: "GET", path: "/api/trips"},
		{method: "POST", path: "/api/trips", body: sampleDoc("x")},
	} {
		rec := do(t, h, c)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d, want 401", c.method, c.path, rec.Code)
		}
	}
}

func TestCreateThenReadByToken(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	if tr.ViewToken == "" || tr.EditToken == "" || tr.ViewToken == tr.EditToken {
		t.Fatalf("bad tokens: %+v", tr)
	}

	rec := do(t, h, call{method: "GET", path: "/api/trips/" + tr.ViewToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	got := decode[struct {
		Access    string          `json:"access"`
		Revision  int             `json:"revision"`
		Doc       json.RawMessage `json:"doc"`
		Log       []any           `json:"log"`
		ViewToken string          `json:"viewToken"`
	}](t, rec)

	if got.Access != "view" {
		t.Errorf("access = %q, want view", got.Access)
	}
	if got.ViewToken != "" {
		t.Error("a non-admin response must not echo the share tokens")
	}
	if rec.Header().Get("ETag") != strconv.Itoa(got.Revision) {
		t.Errorf("ETag %q does not match revision %d", rec.Header().Get("ETag"), got.Revision)
	}
}

func TestUnknownTokenIsNotFound(t *testing.T) {
	h, _ := testServer(t)
	rec := do(t, h, call{method: "GET", path: "/api/trips/0123456789abcdef"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestViewTokenCannotWrite(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{method: "PUT", path: "/api/trips/" + tr.ViewToken, body: sampleDoc("alps")})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403: %s", rec.Code, rec.Body)
	}

	rec = do(t, h, call{
		method: "PUT", path: "/api/trips/" + tr.ViewToken + "/log",
		body: map[string]any{"entries": []any{
			map[string]any{"day": 2, "field": "km", "value": 100, "updatedAt": 1},
		}},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("log write with a view token: got %d, want 403", rec.Code)
	}
}

func TestInvalidDocumentIsRejectedWithFieldPaths(t *testing.T) {
	h, _ := testServer(t)

	doc := sampleDoc("alps")
	days := doc["days"].([]any)
	days[1].(map[string]any)["title"] = ""
	days[1].(map[string]any)["type"] = "cycling"

	rec := do(t, h, call{method: "POST", path: "/api/trips", body: doc, admin: true})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422: %s", rec.Code, rec.Body)
	}

	body := decode[errorBody](t, rec)
	paths := map[string]bool{}
	for _, e := range body.Errors {
		paths[e.Path] = true
	}
	for _, want := range []string{"days[1].title", "days[1].type"} {
		if !paths[want] {
			t.Errorf("no error for %s; got %+v", want, body.Errors)
		}
	}
}

func TestDuplicateSlugRejected(t *testing.T) {
	h, _ := testServer(t)
	createTrip(t, h, "alps")

	rec := do(t, h, call{method: "POST", path: "/api/trips", body: sampleDoc("alps"), admin: true})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422: %s", rec.Code, rec.Body)
	}
	if body := decode[errorBody](t, rec); len(body.Errors) != 1 || body.Errors[0].Path != "slug" {
		t.Errorf("expected a slug error, got %+v", body.Errors)
	}
}

func TestStaleWriteIsRejectedWithCurrentRevision(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	first := sampleDoc("alps")
	first["name"] = "First"
	rec := do(t, h, call{method: "PUT", path: "/api/trips/" + tr.EditToken, body: first,
		headers: map[string]string{"If-Match": strconv.Itoa(tr.Revision)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("first write: %d %s", rec.Code, rec.Body)
	}

	second := sampleDoc("alps")
	second["name"] = "Second"
	rec = do(t, h, call{method: "PUT", path: "/api/trips/" + tr.EditToken, body: second,
		headers: map[string]string{"If-Match": strconv.Itoa(tr.Revision)}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale write: got %d, want 409: %s", rec.Code, rec.Body)
	}
	if body := decode[errorBody](t, rec); body.Revision != tr.Revision+1 {
		t.Errorf("409 should carry the current revision, got %d", body.Revision)
	}
}

func TestExportRoundTrips(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{method: "GET", path: "/api/trips/" + tr.ViewToken + "/export"})
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="alps.json"` {
		t.Errorf("Content-Disposition = %q", cd)
	}

	var doc trip.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("export is not a trip document: %v", err)
	}
	if errs, _ := trip.Validate(&doc); len(errs) > 0 {
		t.Fatalf("exported document does not validate: %v", errs)
	}

	// Push it straight back.
	rec = do(t, h, call{method: "PUT", path: "/api/trips/" + tr.EditToken, body: doc,
		headers: map[string]string{"If-Match": strconv.Itoa(tr.Revision)}})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-import: %d %s", rec.Code, rec.Body)
	}
}

func TestPatchEditsOneField(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method: "PATCH", path: "/api/trips/" + tr.EditToken,
		body: map[string]any{"ops": []any{
			map[string]any{"path": "days[1].km", "value": 245},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}

	got := decode[struct {
		Doc json.RawMessage `json:"doc"`
	}](t, rec)
	var doc trip.Trip
	if err := json.Unmarshal(got.Doc, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Days[1].Km != 245 {
		t.Errorf("km = %v, want 245", doc.Days[1].Km)
	}
	if doc.Days[1].Title != "Loop" {
		t.Errorf("patch disturbed the rest of the day: %+v", doc.Days[1])
	}
}

// A patch must not be able to produce a document a full import would refuse.
func TestPatchCannotBreakTheDocument(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method: "PATCH", path: "/api/trips/" + tr.EditToken,
		body: map[string]any{"ops": []any{
			map[string]any{"path": "days[1].type", "value": "hovercraft"},
		}},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422: %s", rec.Code, rec.Body)
	}
	if body := decode[errorBody](t, rec); len(body.Errors) == 0 || body.Errors[0].Path != "days[1].type" {
		t.Errorf("expected a days[1].type error, got %+v", body.Errors)
	}
}

func TestLogWritesSettleLastWriteWins(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	put := func(km int, at int64) *httptest.ResponseRecorder {
		return do(t, h, call{
			method: "PUT", path: "/api/trips/" + tr.EditToken + "/log",
			body: map[string]any{"entries": []any{
				map[string]any{"day": 2, "field": "km", "value": km, "updatedAt": at},
			}},
		})
	}

	if rec := put(300, 2000); rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	rec := put(100, 1000) // an older write arriving late
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}

	settled := decode[struct {
		Log []store.LogEntry `json:"log"`
	}](t, rec)
	if len(settled.Log) != 1 || string(settled.Log[0].Value) != "300" {
		t.Fatalf("older write won, or duplicated: %+v", settled.Log)
	}
}

func TestUnknownLogFieldRejected(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{
		method: "PUT", path: "/api/trips/" + tr.EditToken + "/log",
		body: map[string]any{"entries": []any{
			map[string]any{"day": 2, "field": "mood", "value": "good", "updatedAt": 1},
		}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestRotateTokensInvalidatesOldLinks(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	rec := do(t, h, call{method: "POST", path: "/api/trips/" + tr.EditToken + "/tokens/rotate", admin: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body)
	}
	rotated := decode[struct {
		ViewToken string `json:"viewToken"`
		EditToken string `json:"editToken"`
	}](t, rec)

	if rotated.ViewToken == tr.ViewToken || rotated.EditToken == tr.EditToken {
		t.Fatal("tokens did not change")
	}
	if rec := do(t, h, call{method: "GET", path: "/api/trips/" + tr.ViewToken}); rec.Code != http.StatusNotFound {
		t.Errorf("old link still works: %d", rec.Code)
	}
	if rec := do(t, h, call{method: "GET", path: "/api/trips/" + rotated.ViewToken}); rec.Code != http.StatusOK {
		t.Errorf("new link does not work: %d", rec.Code)
	}
}

func TestAdminTokenGrantsEditOnAnyTrip(t *testing.T) {
	h, _ := testServer(t)
	tr := createTrip(t, h, "alps")

	// The read-only link, but with the owner's admin token attached.
	rec := do(t, h, call{
		method: "PATCH", path: "/api/trips/" + tr.ViewToken, admin: true,
		body: map[string]any{"ops": []any{
			map[string]any{"path": "name", "value": "Renamed"},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body)
	}
}

func TestSchemaEndpointsServeUsableJSON(t *testing.T) {
	h, _ := testServer(t)

	rec := do(t, h, call{method: "GET", path: "/api/schema/trip.json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("schema: %d", rec.Code)
	}
	var schema map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if schema["$schema"] == nil || schema["properties"] == nil {
		t.Error("schema is missing $schema or properties")
	}

	rec = do(t, h, call{method: "GET", path: "/api/schema/example.json"})
	if rec.Code != http.StatusOK {
		t.Fatalf("example: %d", rec.Code)
	}
	var doc trip.Trip
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("example is not a trip: %v", err)
	}
	if errs, _ := trip.Validate(&doc); len(errs) > 0 {
		t.Errorf("the published example does not validate: %v", errs)
	}
}

func TestShareTokensAreRedactedFromLogPaths(t *testing.T) {
	full := "/api/trips/9f3a2c7e1b4d8e0a5c6b7d8e/log"
	got := redactToken(full)
	if got == full {
		t.Fatal("token left intact in the log path")
	}
	if got != "/api/trips/9f3a…/log" {
		t.Errorf("redacted to %q", got)
	}
}
