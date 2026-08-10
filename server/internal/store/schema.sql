-- The trip is kept whole, as the JSON document it is edited as. Shredding it
-- into tables would buy nothing: it is always read and written entire.
CREATE TABLE IF NOT EXISTS trips (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    doc         TEXT NOT NULL,
    revision    INTEGER NOT NULL DEFAULT 1,
    view_token  TEXT NOT NULL UNIQUE,
    edit_token  TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- One row per field rather than per day, so an offline queue flushes as a set
-- of independent upserts and two riders editing different fields of the same
-- day never clobber each other.
CREATE TABLE IF NOT EXISTS log_entries (
    trip_id    TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    day        INTEGER NOT NULL,
    field      TEXT NOT NULL,
    value      TEXT,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (trip_id, day, field)
);

CREATE TABLE IF NOT EXISTS kit_state (
    trip_id    TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    item_id    TEXT NOT NULL,
    checked    INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (trip_id, item_id)
);

-- Shared route cache. One person fetches a day, the whole group gets it, and
-- an offline browser still has the geometry.
CREATE TABLE IF NOT EXISTS routes (
    trip_id    TEXT NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    day        INTEGER NOT NULL,
    signature  TEXT NOT NULL,
    polyline   TEXT NOT NULL,
    km         REAL NOT NULL,
    hours      REAL NOT NULL,
    fetched_at TEXT NOT NULL,
    PRIMARY KEY (trip_id, day)
);
