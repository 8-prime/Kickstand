package store

import (
	"context"
	"database/sql"
	"fmt"

	"bike-trip/server/internal/trip"
)

// Tables whose rows belong to a day and therefore move when days are
// renumbered. Both use (trip_id, day) as their key, so both need the same
// treatment and neither may be forgotten.
var dayKeyedTables = []string{"log_entries", "routes"}

// ReplaceDocAndRemap writes a document whose days have been renumbered, and
// moves the rows keyed by the old numbers with it.
//
// remap is keyed by the old day number; 0 means the day was deleted. Both
// happen in one transaction: a log entry that outlived its day, or one left
// pointing at whichever day inherited its number, is worse than the edit
// failing outright.
func (s *Store) ReplaceDocAndRemap(
	ctx context.Context,
	id string,
	doc *trip.Trip,
	ifRevision int,
	remap map[int]int,
) (*Trip, error) {
	return s.replaceDoc(ctx, id, doc, ifRevision, func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range dayKeyedTables {
			if err := remapDays(ctx, tx, table, id, remap); err != nil {
				return err
			}
		}
		return nil
	})
}

// remapDays moves one table's rows onto their new day numbers.
//
// It goes via negative day numbers rather than updating in place. Renumbering
// is a permutation, so an in-place update collides with a row that has not
// moved yet — parking every mover below zero first means nothing lands on an
// occupied key.
func remapDays(ctx context.Context, tx *sql.Tx, table, tripID string, remap map[int]int) error {
	// A row whose day is not in the remap at all is already an orphan: it was
	// logged against a day this document does not have. Clear it out here
	// rather than leave it to collide with a day that is about to take its
	// number.
	keep := make([]any, 0, len(remap)+1)
	keep = append(keep, tripID)
	placeholders := ""
	for old, now := range remap {
		if now == 0 {
			continue
		}
		if placeholders != "" {
			placeholders += ","
		}
		placeholders += "?"
		keep = append(keep, old)
	}
	del := fmt.Sprintf(`DELETE FROM %s WHERE trip_id = ?`, table)
	if placeholders != "" {
		del += fmt.Sprintf(` AND day NOT IN (%s)`, placeholders)
	}
	if _, err := tx.ExecContext(ctx, del, keep...); err != nil {
		return fmt.Errorf("drop stale %s rows: %w", table, err)
	}

	park := fmt.Sprintf(`UPDATE %s SET day = -day WHERE trip_id = ? AND day = ?`, table)
	land := fmt.Sprintf(`UPDATE %s SET day = ? WHERE trip_id = ? AND day = ?`, table)

	for old, now := range remap {
		if now == 0 || now == old {
			continue
		}
		if _, err := tx.ExecContext(ctx, park, tripID, old); err != nil {
			return fmt.Errorf("park %s rows for day %d: %w", table, old, err)
		}
	}
	for old, now := range remap {
		if now == 0 || now == old {
			continue
		}
		if _, err := tx.ExecContext(ctx, land, now, tripID, -old); err != nil {
			return fmt.Errorf("move %s rows from day %d to %d: %w", table, old, now, err)
		}
	}
	return nil
}
