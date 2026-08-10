package trip

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Op sets one field of a document, addressed the same way the validator
// reports errors: `campsites[0].phone`, `days[4].km`, `kit[1].items[2].title`.
//
// Field-level ops rather than whole-document writes exist for one reason: an
// offline client queues them, and two people editing different fields of the
// same trip must both survive the flush. A queued whole document would
// silently discard the other person's change.
type Op struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// ApplyPatch applies ops to a document.
//
// It works on the decoded JSON rather than the Go struct so a path can point
// anywhere without a hand-written setter per field. The result is re-decoded
// and revalidated by the caller, so a patch cannot smuggle in a document the
// validator would have rejected.
//
// Setting is the only operation. To add or remove a list item, set the whole
// list — the client already holds the document.
func ApplyPatch(doc []byte, ops []Op) ([]byte, []FieldError) {
	var root any
	if err := json.Unmarshal(doc, &root); err != nil {
		return nil, []FieldError{{Path: "", Message: "the stored document is not valid JSON"}}
	}

	var errs []FieldError
	for _, op := range ops {
		segs, err := parsePath(op.Path)
		if err != nil {
			errs = append(errs, FieldError{Path: op.Path, Message: err.Error()})
			continue
		}
		var value any
		if err := json.Unmarshal(op.Value, &value); err != nil {
			errs = append(errs, FieldError{Path: op.Path, Message: "value is not valid JSON"})
			continue
		}
		if err := setPath(root, segs, value); err != nil {
			errs = append(errs, FieldError{Path: op.Path, Message: err.Error()})
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}

	out, err := json.Marshal(root)
	if err != nil {
		return nil, []FieldError{{Path: "", Message: "could not re-encode the patched document"}}
	}
	return out, nil
}

type segment struct {
	key   string
	index int
	isIdx bool
}

// parsePath splits `days[4].stops[2].lon` into its segments.
func parsePath(path string) ([]segment, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("empty path")
	}

	var segs []segment
	for _, part := range strings.Split(path, ".") {
		name := part
		// A part may carry any number of trailing indices: items[2][0].
		for {
			open := strings.IndexByte(name, '[')
			if open < 0 {
				break
			}
			close := strings.IndexByte(name[open:], ']')
			if close < 0 {
				return nil, fmt.Errorf("unclosed [ in %q", part)
			}
			close += open

			if head := name[:open]; head != "" {
				segs = append(segs, segment{key: head})
			}
			n, err := strconv.Atoi(name[open+1 : close])
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%q is not a list index", name[open+1:close])
			}
			segs = append(segs, segment{index: n, isIdx: true})
			name = name[close+1:]
		}
		if name != "" {
			segs = append(segs, segment{key: name})
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	return segs, nil
}

func setPath(root any, segs []segment, value any) error {
	current := root

	for i, seg := range segs {
		last := i == len(segs)-1

		if seg.isIdx {
			arr, ok := current.([]any)
			if !ok {
				return fmt.Errorf("%s is not a list", pathTo(segs, i))
			}
			if seg.index >= len(arr) {
				return fmt.Errorf("index %d is past the end of %s (%d items)",
					seg.index, pathTo(segs, i), len(arr))
			}
			if last {
				arr[seg.index] = value
				return nil
			}
			current = arr[seg.index]
			continue
		}

		obj, ok := current.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not an object", pathTo(segs, i))
		}
		if last {
			obj[seg.key] = value
			return nil
		}
		next, exists := obj[seg.key]
		if !exists {
			return fmt.Errorf("no field %q in %s", seg.key, pathTo(segs, i))
		}
		current = next
	}
	return nil
}

// pathTo renders the path up to (not including) segment i, for error messages.
func pathTo(segs []segment, i int) string {
	if i == 0 {
		return "the document"
	}
	var b strings.Builder
	for _, s := range segs[:i] {
		if s.isIdx {
			fmt.Fprintf(&b, "[%d]", s.index)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s.key)
	}
	return b.String()
}
