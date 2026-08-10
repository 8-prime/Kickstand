package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// LogEntry is one field of one day's log: the distance actually ridden, the
// weather mark, or the note.
//
// Value is JSON so a number, a string and null all round-trip without the
// store caring which fields exist. UpdatedAt is the client's clock in epoch
// milliseconds, and is what decides a conflict.
type LogEntry struct {
	Day       int             `json:"day"`
	Field     string          `json:"field"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt int64           `json:"updatedAt"`
}

// KitEntry is one checklist item's tick state.
type KitEntry struct {
	ItemID    string `json:"itemId"`
	Checked   bool   `json:"checked"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Fields the log accepts. Anything else is a client bug and is rejected
// rather than stored, so a typo cannot quietly accumulate rows forever.
var logFields = map[string]bool{"km": true, "wx": true, "note": true}

// ValidLogField reports whether a field name is one the log stores.
func ValidLogField(name string) bool { return logFields[name] }

// Log returns every log entry for a trip.
func (s *Store) Log(ctx context.Context, tripID string) ([]LogEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, field, value, updated_at FROM log_entries WHERE trip_id = ? ORDER BY day, field`,
		tripID)
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	defer rows.Close()

	out := []LogEntry{}
	for rows.Next() {
		var (
			e   LogEntry
			val []byte
		)
		if err := rows.Scan(&e.Day, &e.Field, &val, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan log entry: %w", err)
		}
		e.Value = val
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertLog applies a batch of log writes, last write wins per field.
//
// The batch is what an offline client flushes on reconnect, so it has to be
// idempotent and order-independent: an entry older than what is already
// stored is dropped, not applied. Replaying the same batch twice changes
// nothing.
func (s *Store) UpsertLog(ctx context.Context, tripID string, entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO log_entries (trip_id, day, field, value, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (trip_id, day, field) DO UPDATE SET
		   value = excluded.value, updated_at = excluded.updated_at
		 WHERE excluded.updated_at >= log_entries.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare log upsert: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		if !ValidLogField(e.Field) {
			return fmt.Errorf("unknown log field %q", e.Field)
		}
		if _, err := stmt.ExecContext(ctx, tripID, e.Day, e.Field, string(e.Value), e.UpdatedAt); err != nil {
			return fmt.Errorf("upsert log day %d %s: %w", e.Day, e.Field, err)
		}
	}
	return tx.Commit()
}

// ClearLog removes every logged distance, weather mark and note for a trip.
// The checklist is left alone: they are cleared separately on purpose.
func (s *Store) ClearLog(ctx context.Context, tripID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM log_entries WHERE trip_id = ?`, tripID)
	if err != nil {
		return fmt.Errorf("clear log: %w", err)
	}
	return nil
}

// KitState returns every checklist tick for a trip.
func (s *Store) KitState(ctx context.Context, tripID string) ([]KitEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_id, checked, updated_at FROM kit_state WHERE trip_id = ? ORDER BY item_id`,
		tripID)
	if err != nil {
		return nil, fmt.Errorf("read kit state: %w", err)
	}
	defer rows.Close()

	out := []KitEntry{}
	for rows.Next() {
		var e KitEntry
		if err := rows.Scan(&e.ItemID, &e.Checked, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan kit entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertKit applies checklist ticks, last write wins per item.
func (s *Store) UpsertKit(ctx context.Context, tripID string, entries []KitEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO kit_state (trip_id, item_id, checked, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (trip_id, item_id) DO UPDATE SET
		   checked = excluded.checked, updated_at = excluded.updated_at
		 WHERE excluded.updated_at >= kit_state.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare kit upsert: %w", err)
	}
	defer stmt.Close()

	for _, e := range entries {
		if e.ItemID == "" {
			return fmt.Errorf("kit entry with no item id")
		}
		if _, err := stmt.ExecContext(ctx, tripID, e.ItemID, e.Checked, e.UpdatedAt); err != nil {
			return fmt.Errorf("upsert kit %s: %w", e.ItemID, err)
		}
	}
	return tx.Commit()
}

// ClearKit unticks everything for a trip, leaving the ride log alone.
func (s *Store) ClearKit(ctx context.Context, tripID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM kit_state WHERE trip_id = ?`, tripID)
	if err != nil {
		return fmt.Errorf("clear kit state: %w", err)
	}
	return nil
}
