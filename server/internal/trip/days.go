package trip

import "fmt"

// DayOp is a structural change to the day list: one added, removed, or moved.
//
// This is not a patch. Day numbers must run 1..N with no holes — the validator
// insists on it and the spine draws it — so touching the list renumbers every
// day after the change. The log and the route cache are both keyed by day
// number, which means the renumbering has to be reported back to whoever holds
// those keys rather than quietly applied to the document alone. Remap is what
// carries it.
type DayOp struct {
	// "insert", "delete" or "move".
	Op string `json:"op"`
	// The day to delete or move.
	Day int `json:"day"`
	// insert: the day to place the new one after. 0 puts it first.
	After int `json:"after"`
	// move: the number the day should end up with.
	To int `json:"to"`
}

// ApplyDayOp rewrites t.Days for the operation and returns the renumbering.
//
// The map is keyed by the old day number; a value of 0 means the day is gone.
// Bases are carried across with it, and a base whose arrival day was deleted
// comes back as a warning rather than an error — losing a day should not
// require also knowing which base pointed at it.
func ApplyDayOp(t *Trip, op DayOp) (remap map[int]int, warnings []FieldError, err error) {
	if len(t.Days) == 0 {
		return nil, nil, fmt.Errorf("this trip has no days to change")
	}

	// Positions hold the old day number of each slot, or 0 for a day that did
	// not exist before. Working in this space keeps the reordering and the
	// renumbering as one step rather than two that can disagree.
	order := make([]int, 0, len(t.Days)+1)
	byNumber := make(map[int]Day, len(t.Days))
	for _, d := range t.Days {
		order = append(order, d.N)
		byNumber[d.N] = d
	}

	inserted := Day{}

	switch op.Op {
	case "insert":
		if op.After != 0 {
			if _, ok := byNumber[op.After]; !ok {
				return nil, nil, fmt.Errorf("there is no day %d to insert after", op.After)
			}
		}
		inserted = Day{
			Type:  Ride,
			Title: "New day",
			// Never nil: the client maps over this list, and a null would be a
			// crash rather than an empty day.
			Stops: []Stop{},
		}
		order = insertAt(order, indexOf(order, op.After)+1, 0)

	case "delete":
		i := indexOf(order, op.Day)
		if i < 0 {
			return nil, nil, fmt.Errorf("there is no day %d to delete", op.Day)
		}
		if len(order) == 1 {
			return nil, nil, fmt.Errorf("a trip needs at least one day")
		}
		order = append(order[:i:i], order[i+1:]...)

	case "move":
		i := indexOf(order, op.Day)
		if i < 0 {
			return nil, nil, fmt.Errorf("there is no day %d to move", op.Day)
		}
		to := clamp(op.To, 1, len(order))
		moved := order[i]
		order = append(order[:i:i], order[i+1:]...)
		order = insertAt(order, to-1, moved)

	default:
		return nil, nil, fmt.Errorf("%q is not a day operation; use insert, delete or move", op.Op)
	}

	days := make([]Day, len(order))
	remap = make(map[int]int, len(t.Days))
	for _, d := range t.Days {
		remap[d.N] = 0 // dropped unless a slot claims it below
	}
	for i, old := range order {
		n := i + 1
		if old == 0 {
			inserted.N = n
			days[i] = inserted
			continue
		}
		d := byNumber[old]
		d.N = n
		days[i] = d
		remap[old] = n
	}
	t.Days = days

	for i := range t.Bases {
		was := t.Bases[i].ArriveDay
		switch now := remap[was]; {
		case now > 0:
			t.Bases[i].ArriveDay = now
		default:
			// The day it pitched on is gone. Fall to the next day that
			// survived, or the last day of the trip.
			t.Bases[i].ArriveDay = nextSurviving(remap, was, len(days))
			warnings = append(warnings, FieldError{
				Path: fmt.Sprintf("bases[%d].arriveDay", i),
				Message: fmt.Sprintf("day %d was removed; %s now arrives on day %d — check that is right",
					was, t.Bases[i].Name, t.Bases[i].ArriveDay),
			})
		}
	}

	return remap, warnings, nil
}

// nextSurviving finds a sensible replacement for a day number that is gone:
// the earliest later day that is still in the trip, or the last day.
func nextSurviving(remap map[int]int, from, lastDay int) int {
	best := 0
	for old, now := range remap {
		if now == 0 || old < from {
			continue
		}
		if best == 0 || now < best {
			best = now
		}
	}
	if best == 0 {
		return lastDay
	}
	return best
}

func indexOf(order []int, n int) int {
	for i, v := range order {
		if v == n {
			return i
		}
	}
	return -1
}

func insertAt(order []int, i, v int) []int {
	i = clamp(i, 0, len(order))
	out := make([]int, 0, len(order)+1)
	out = append(out, order[:i]...)
	out = append(out, v)
	return append(out, order[i:]...)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
