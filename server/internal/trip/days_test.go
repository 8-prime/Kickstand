package trip

import "testing"

func fourDayTrip() *Trip {
	return &Trip{
		SchemaVersion: 1,
		Slug:          "test",
		Name:          "Test",
		Bounds:        Bounds{Lat0: 43, Lat1: 44, Lon0: 5, Lon1: 7},
		Bases: []Base{
			{Index: 1, Name: "First", Lat: 43.5, Lon: 6, ArriveDay: 1},
			{Index: 2, Name: "Second", Lat: 43.7, Lon: 6.4, ArriveDay: 3},
		},
		Days: []Day{
			{N: 1, Type: Van, Title: "Out", Van: 900},
			{N: 2, Type: Ride, Title: "Loop", Km: 200},
			{N: 3, Type: Ride, Title: "Gorge", Km: 180},
			{N: 4, Type: Van, Title: "Home", Van: 800},
		},
	}
}

func numbers(t *Trip) []int {
	out := make([]int, len(t.Days))
	for i, d := range t.Days {
		out[i] = d.N
	}
	return out
}

func titles(tr *Trip) []string {
	out := make([]string, len(tr.Days))
	for i, d := range tr.Days {
		out[i] = d.Title
	}
	return out
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInsertRenumbersAndReportsTheMove(t *testing.T) {
	tr := fourDayTrip()

	remap, warnings, err := ApplyDayOp(tr, DayOp{Op: "insert", After: 2})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("insert should not warn, got %v", warnings)
	}

	if want := []int{1, 2, 3, 4, 5}; !equal(numbers(tr), want) {
		t.Fatalf("day numbers = %v, want %v", numbers(tr), want)
	}
	if want := []string{"Out", "Loop", "New day", "Gorge", "Home"}; !equal(titles(tr), want) {
		t.Fatalf("titles = %v, want %v", titles(tr), want)
	}
	// The days after the new one moved, and the caller has to be told so it can
	// move the log and the routes with them.
	for old, now := range map[int]int{1: 1, 2: 2, 3: 4, 4: 5} {
		if remap[old] != now {
			t.Errorf("remap[%d] = %d, want %d", old, remap[old], now)
		}
	}
	// The base that pitched on day 3 still pitches on the same day, now day 4.
	if tr.Bases[1].ArriveDay != 4 {
		t.Errorf("base 2 arriveDay = %d, want 4", tr.Bases[1].ArriveDay)
	}

	if errs, _ := Validate(tr); len(errs) > 0 {
		t.Fatalf("insert produced an invalid trip: %v", errs)
	}
}

func TestInsertAtTheFront(t *testing.T) {
	tr := fourDayTrip()

	if _, _, err := ApplyDayOp(tr, DayOp{Op: "insert", After: 0}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if tr.Days[0].Title != "New day" {
		t.Fatalf("first day = %q, want the new one", tr.Days[0].Title)
	}
	// Never nil: the client maps over this list.
	if tr.Days[0].Stops == nil {
		t.Fatal("a new day must have an empty stop list, not a null one")
	}
	if tr.Bases[0].ArriveDay != 2 {
		t.Errorf("base 1 arriveDay = %d, want 2", tr.Bases[0].ArriveDay)
	}
}

func TestDeleteRenumbersAndClampsAnOrphanedBase(t *testing.T) {
	tr := fourDayTrip()

	remap, warnings, err := ApplyDayOp(tr, DayOp{Op: "delete", Day: 3})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	if want := []int{1, 2, 3}; !equal(numbers(tr), want) {
		t.Fatalf("day numbers = %v, want %v", numbers(tr), want)
	}
	if want := []string{"Out", "Loop", "Home"}; !equal(titles(tr), want) {
		t.Fatalf("titles = %v, want %v", titles(tr), want)
	}
	if remap[3] != 0 {
		t.Errorf("remap[3] = %d, want 0 for a deleted day", remap[3])
	}
	if remap[4] != 3 {
		t.Errorf("remap[4] = %d, want 3", remap[4])
	}

	// The base pitched on the day that is gone. It has to land somewhere, and
	// the rider has to be told where.
	if tr.Bases[1].ArriveDay != 3 {
		t.Errorf("base 2 arriveDay = %d, want 3", tr.Bases[1].ArriveDay)
	}
	if len(warnings) != 1 {
		t.Fatalf("want one warning about the moved base, got %v", warnings)
	}
	if warnings[0].Path != "bases[1].arriveDay" {
		t.Errorf("warning path = %q", warnings[0].Path)
	}

	if errs, _ := Validate(tr); len(errs) > 0 {
		t.Fatalf("delete produced an invalid trip: %v", errs)
	}
}

func TestDeleteTheLastRemainingDayIsRefused(t *testing.T) {
	tr := &Trip{Days: []Day{{N: 1, Type: Ride, Title: "Only"}}}
	if _, _, err := ApplyDayOp(tr, DayOp{Op: "delete", Day: 1}); err == nil {
		t.Fatal("deleting the only day should be refused")
	}
}

func TestMoveReordersWithoutLosingADay(t *testing.T) {
	tr := fourDayTrip()

	remap, _, err := ApplyDayOp(tr, DayOp{Op: "move", Day: 4, To: 2})
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	if want := []string{"Out", "Home", "Loop", "Gorge"}; !equal(titles(tr), want) {
		t.Fatalf("titles = %v, want %v", titles(tr), want)
	}
	if want := []int{1, 2, 3, 4}; !equal(numbers(tr), want) {
		t.Fatalf("day numbers = %v, want %v", numbers(tr), want)
	}
	for old, now := range map[int]int{1: 1, 2: 3, 3: 4, 4: 2} {
		if remap[old] != now {
			t.Errorf("remap[%d] = %d, want %d", old, remap[old], now)
		}
	}
	// Base 2 arrived on old day 3, which is now day 4.
	if tr.Bases[1].ArriveDay != 4 {
		t.Errorf("base 2 arriveDay = %d, want 4", tr.Bases[1].ArriveDay)
	}

	if errs, _ := Validate(tr); len(errs) > 0 {
		t.Fatalf("move produced an invalid trip: %v", errs)
	}
}

func TestMoveBeyondTheEndClamps(t *testing.T) {
	tr := fourDayTrip()
	if _, _, err := ApplyDayOp(tr, DayOp{Op: "move", Day: 1, To: 99}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if tr.Days[3].Title != "Out" {
		t.Fatalf("last day = %q, want the moved one", tr.Days[3].Title)
	}
}

func TestUnknownDaysAndOpsAreRefused(t *testing.T) {
	for _, op := range []DayOp{
		{Op: "delete", Day: 9},
		{Op: "move", Day: 9, To: 1},
		{Op: "insert", After: 9},
		{Op: "shuffle"},
	} {
		if _, _, err := ApplyDayOp(fourDayTrip(), op); err == nil {
			t.Errorf("%+v should have been refused", op)
		}
	}
}
