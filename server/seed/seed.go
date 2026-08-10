// Package seed holds the trip documents the server starts life with.
//
// These are the two plans that used to live as TypeScript modules in the
// frontend. They are loaded once, into an empty database, and are ordinary
// trips from that moment on — editable, deletable, and not consulted again.
package seed

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"

	"bike-trip/server/internal/trip"
)

//go:embed *.json
var files embed.FS

// Documents returns the built-in trips, sorted by filename so seeding order
// is the same on every machine.
func Documents() ([]*trip.Trip, error) {
	names, err := fs.Glob(files, "*.json")
	if err != nil {
		return nil, fmt.Errorf("list seed files: %w", err)
	}
	sort.Strings(names)

	out := make([]*trip.Trip, 0, len(names))
	for _, name := range names {
		raw, err := files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var doc trip.Trip
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if errs, _ := trip.Validate(&doc); len(errs) > 0 {
			return nil, fmt.Errorf("seed %s is invalid: %v", name, errs)
		}
		trip.Normalize(&doc)
		out = append(out, &doc)
	}
	return out, nil
}
