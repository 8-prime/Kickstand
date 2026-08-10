package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"bike-trip/server/internal/trip"
)

var (
	// ErrNotFound covers both "no such trip" and "that token means nothing",
	// deliberately: a caller guessing tokens learns nothing from the error.
	ErrNotFound = errors.New("not found")

	// ErrRevisionMismatch means someone else saved while you were editing.
	ErrRevisionMismatch = errors.New("revision mismatch")

	// ErrSlugTaken means another trip already uses that slug.
	ErrSlugTaken = errors.New("slug already in use")
)

// Access is what a token grants.
type Access string

const (
	AccessView Access = "view"
	AccessEdit Access = "edit"
)

// CanEdit reports whether this access level may write.
func (a Access) CanEdit() bool { return a == AccessEdit }

// Trip is a stored trip: the document plus the bookkeeping around it.
type Trip struct {
	ID        string          `json:"id"`
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Doc       json.RawMessage `json:"doc"`
	Revision  int             `json:"revision"`
	ViewToken string          `json:"viewToken,omitempty"`
	EditToken string          `json:"editToken,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// Summary is the listing view. It never carries tokens — the list is
// unauthenticated, and a trip's links are not something to hand out with it.
type Summary struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Subtitle  string    `json:"subtitle,omitempty"`
	Dates     string    `json:"dates,omitempty"`
	Days      int       `json:"days"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListTrips returns every trip, newest change first.
func (s *Store) ListTrips(ctx context.Context) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, slug, name, doc, updated_at FROM trips ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list trips: %w", err)
	}
	defer rows.Close()

	out := []Summary{}
	for rows.Next() {
		var (
			sum     Summary
			doc     []byte
			updated string
		)
		if err := rows.Scan(&sum.ID, &sum.Slug, &sum.Name, &doc, &updated); err != nil {
			return nil, fmt.Errorf("scan trip: %w", err)
		}
		sum.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

		// Pull the handful of display fields out of the document rather than
		// denormalising them into columns that would then need keeping in sync.
		var meta struct {
			Subtitle string     `json:"subtitle"`
			Dates    string     `json:"dates"`
			Days     []struct{} `json:"days"`
		}
		if err := json.Unmarshal(doc, &meta); err == nil {
			sum.Subtitle = meta.Subtitle
			sum.Dates = meta.Dates
			sum.Days = len(meta.Days)
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

// CreateTrip stores a new trip and mints its share tokens.
func (s *Store) CreateTrip(ctx context.Context, doc *trip.Trip) (*Trip, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode document: %w", err)
	}

	id, err := newToken()
	if err != nil {
		return nil, err
	}
	viewTok, err := newToken()
	if err != nil {
		return nil, err
	}
	editTok, err := newToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO trips (id, slug, name, doc, revision, view_token, edit_token, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?)`,
		id, doc.Slug, doc.Name, string(raw), viewTok, editTok, stamp, stamp)
	if err != nil {
		if isUniqueViolation(err, "slug") {
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("insert trip: %w", err)
	}

	return &Trip{
		ID: id, Slug: doc.Slug, Name: doc.Name, Doc: raw, Revision: 1,
		ViewToken: viewTok, EditToken: editTok, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// TripByID loads a trip by its internal id.
func (s *Store) TripByID(ctx context.Context, id string) (*Trip, error) {
	return s.scanTrip(s.db.QueryRowContext(ctx, selectTrip+` WHERE id = ?`, id))
}

// TripByToken resolves a share token to a trip and the access it grants.
//
// Both tokens are checked in one query so a wrong token takes the same path,
// and the same amount of time, as a right one.
func (s *Store) TripByToken(ctx context.Context, token string) (*Trip, Access, error) {
	if token == "" {
		return nil, "", ErrNotFound
	}
	t, err := s.scanTrip(s.db.QueryRowContext(ctx,
		selectTrip+` WHERE view_token = ? OR edit_token = ?`, token, token))
	if err != nil {
		return nil, "", err
	}
	if token == t.EditToken {
		return t, AccessEdit, nil
	}
	return t, AccessView, nil
}

// ReplaceDoc writes a new version of the document.
//
// ifRevision guards against a lost update: pass the revision you read, and if
// someone else has saved since, you get ErrRevisionMismatch instead of
// silently overwriting them. Pass 0 to force.
//
// Tokens, log entries and cached routes are untouched — replacing the plan
// does not throw away what people have written against it.
func (s *Store) ReplaceDoc(ctx context.Context, id string, doc *trip.Trip, ifRevision int) (*Trip, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode document: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var current int
	err = tx.QueryRowContext(ctx, `SELECT revision FROM trips WHERE id = ?`, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read revision: %w", err)
	}
	if ifRevision != 0 && ifRevision != current {
		return nil, ErrRevisionMismatch
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx,
		`UPDATE trips SET slug = ?, name = ?, doc = ?, revision = revision + 1, updated_at = ?
		 WHERE id = ?`,
		doc.Slug, doc.Name, string(raw), stamp, id)
	if err != nil {
		if isUniqueViolation(err, "slug") {
			return nil, ErrSlugTaken
		}
		return nil, fmt.Errorf("update trip: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.TripByID(ctx, id)
}

// RotateTokens issues new share links and invalidates the old ones.
func (s *Store) RotateTokens(ctx context.Context, id string) (*Trip, error) {
	viewTok, err := newToken()
	if err != nil {
		return nil, err
	}
	editTok, err := newToken()
	if err != nil {
		return nil, err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE trips SET view_token = ?, edit_token = ?, updated_at = ? WHERE id = ?`,
		viewTok, editTok, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return nil, fmt.Errorf("rotate tokens: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.TripByID(ctx, id)
}

// DeleteTrip removes a trip and everything logged against it.
func (s *Store) DeleteTrip(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM trips WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete trip: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountTrips reports how many trips exist. Used to decide whether to seed.
func (s *Store) CountTrips(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trips`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count trips: %w", err)
	}
	return n, nil
}

const selectTrip = `SELECT id, slug, name, doc, revision, view_token, edit_token, created_at, updated_at FROM trips`

func (s *Store) scanTrip(row *sql.Row) (*Trip, error) {
	var (
		t                Trip
		doc              []byte
		created, updated string
	)
	err := row.Scan(&t.ID, &t.Slug, &t.Name, &doc, &t.Revision,
		&t.ViewToken, &t.EditToken, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan trip: %w", err)
	}
	t.Doc = doc
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &t, nil
}

// Document decodes the stored JSON back into a Trip document.
func (t *Trip) Document() (*trip.Trip, error) {
	var doc trip.Trip
	if err := json.Unmarshal(t.Doc, &doc); err != nil {
		return nil, fmt.Errorf("decode stored document: %w", err)
	}
	return &doc, nil
}

// The driver returns constraint failures as text; there is no typed error to
// match on, so the column name is what distinguishes them.
func isUniqueViolation(err error, column string) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") && strings.Contains(msg, column)
}
