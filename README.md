# Motorcycle trip planner

Plan a motorcycle trip: days, base camps, campsites, a kit checklist, and a roadbook you log
against once you are riding. A trip is one JSON document — write it by hand, or have a model
write it — stored by a small Go service that also holds the shared log and hands out links.

Two plans ship with it: **Brittany & Normandy** and **Provence & the Alpes-Maritimes**, eleven
days each, bikes plus a transporter van, September 2026.

## Run it

```bash
# Development: two processes, Vite proxies /api to the Go server.
make dev-api          # or: cd server && go run .
make dev-web          # or: pnpm dev          → http://localhost:5173

# Release: one binary with the app inside it.
make build            # → server/server, serves everything on :8080
```

First run seeds the two built-in trips and prints an **admin token**. Keep it — it is what
lets you list, create and delete trips. Set `BIKETRIP_ADMIN_TOKEN` to fix it across restarts.

| Flag | Env | Default |
|---|---|---|
| `-addr` | `BIKETRIP_ADDR` | `:8080` |
| `-db` | `BIKETRIP_DB` | `biketrip.db` |
| `-admin-token` | `BIKETRIP_ADMIN_TOKEN` | generated per run |
| `-allow-origin` | `BIKETRIP_ALLOW_ORIGIN` | same-origin only |
| `-nominatim` | `BIKETRIP_NOMINATIM` | `https://nominatim.openstreetmap.org` |
| `-no-seed` | — | seeds on an empty database |

## The trip document

Everything about a trip lives in one JSON file: metadata, base camps, days with their stops,
campsites and the checklist. Import and export are how a whole plan moves; the UI edits it in
place — see [Editing](#editing).

```jsonc
{
  "schemaVersion": 1,
  "slug": "provence",
  "name": "Alpes Maritimes",
  "bounds": { "lat0": 43.0, "lat1": 44.4, "lon0": 4.9, "lon1": 7.8 },
  "bases": [{ "index": 1, "name": "Sospel", "lat": 43.876, "lon": 7.448, "arriveDay": 1 }],
  "days": [{
    "n": 5, "type": "ride", "title": "Gorges du Verdon circuit",
    "km": 230, "hours": 5.5,
    "stops": [{ "name": "Castellane", "lat": 43.847, "lon": 6.513 }]
  }],
  "campsites": [...], "kit": [...]
}
```

Stops are objects, not `[name, lat, lon]` triples, specifically because a positional triple
invites a swapped latitude and longitude that nothing catches but a map that looks wrong.

### Having a model write one

```
GET /api/schema/trip.json     JSON Schema, fully annotated
GET /api/schema/example.json  a complete real trip
```

Give a model both and ask for a trip in the same shape, then paste it into **Trip → Add a
trip**. Anything wrong comes back as a list of fields to fix rather than a failed import:

```
slug                "Pyrenees 2027" must be lowercase letters, digits and single hyphens
days[1].title       required
days[2].type        "cycling" is not a day type; use "ride" or "van"
days                day 3 is missing — days must run 1 to 3
bases[0].arriveDay  14 is beyond the 3 days in this trip
kit[0].items[1].id  duplicate id "doc-passport", already used at kit[0].items[0]
```

Every problem is reported at once, with a path, because the caller is usually a model that
should get the whole list to fix in one pass. Warnings — a stop outside the map bounds, a day
with one stop — never block an import.

Adding a trip needs no code changes.

## Sharing

Each trip has two unlisted links. There are no accounts.

```
/t/<viewToken>    read: map, roadbook, checklist, campsites
/t/<editToken>    write: log distances, edit fields, replace the document
```

Anyone with a link is in, so treat the edit link as the credential it is. **Trip → Issue new
links** revokes both.

The trip *list* is gated behind the admin token, deliberately: it carries every trip's links,
so without a gate the links would protect nothing.

## Offline

The trip and its route geometry are cached in IndexedDB and the app shell in a service worker,
so it opens and works at a campsite with no signal — including after closing the tab. Anything
you log is queued and pushed when the connection returns.

Writes are field-level (`{day, field, value, updatedAt}` and `{path, value}` patch ops), not
whole documents, so two people editing different things while offline both survive the flush.
Conflicts settle last-write-wins per field.

## Editing

Holding an edit link, every field is editable where it is shown. Three kinds of edit, with
three different mechanisms, because they have three different failure modes.

**Fields** — click the value, change it, it saves. One `{path, value}` op against
`PATCH /api/trips/:token`. Several fields that belong to one gesture go up as one op list and
therefore one revision: dragging a stop writes its coordinates and, if it landed off the edge
of the map window, the widened `bounds` with it. These queue offline and merge per field.

**Stops** — the shape of a day, and the thing everything else is derived from. Turn on *Place*
on the map and the day's stops become handles: drag one to move it, click empty map to add one
at the end, click a stop to rename or remove it. Base-camp and campsite pins drag in the same
mode; dragging a campsite pin onto the actual pitch is what clears its `coordsApprox` flag. The
same stops are a reorderable list in the day panel, with a search box that looks names up
against OSM. Order is what the router is asked to go through, so reordering is how a day gets
reshaped. Every change writes the whole `stops` array as one op — what the patcher asks for,
and what makes a burst of drags collapse to the last one in the offline queue.

**Days** — adding, deleting and moving one. This is the only edit that is *not* a patch, and
`POST /api/trips/:token/days` exists for it. Day numbers are keys, not labels: `log_entries`
and `routes` are both stored against them, and the validator requires them to run 1..N with no
holes. Deleting day 4 of 11 renumbers 5..11 down one, so the document edit and the moving of
every row keyed by those numbers happen in one transaction. A base whose arrival day was
deleted is moved to the next surviving day and you are told so. Day edits need a connection —
there is no honest way to replay a renumbering against someone else's version later.

Place lookup runs on the server for the same reason routing does, plus one of its own: the OSM
usage policy requires an identifying `User-Agent`, and a browser cannot send one. Results are
biased toward the trip's `bounds` and cached per query. A geocoder that is down costs you a
stop named `48.2513, -4.4721`, not a lost click.

## Routing

Road geometry comes from the public OSRM demo server, fetched **by the Go service**, not the
browser: one fetch serves the whole group, the demo server sees one polite client pacing
itself, and offline browsers get the geometry with the trip. Editing a day's stops schedules a
refresh for that day a beat later, so the drawn road catches up on its own.

Routed distance and planned distance disagree on purpose. OSRM returns the *shortest* drive
through a day's stops; the plans assume the scenic line. Brittany day 5 routes at 99 km against
320 planned. **Fuel and time against the planned figure.** That is why the routed figure is
offered as a button — *use 214 km · 4.2 h* — rather than filled in for you: taking it is a
decision that the day is no longer going the long way round.

Route geometry is invalidated per day by a hash of that day's stop coordinates, so moving a
stop refetches that day and nothing else. Until the refetch lands, a cached line whose ends no
longer match the day's first and last stop is drawn as the dashed schematic instead — a broken
line that says it is not the road beats a real road through somewhere you are not going.

## Layout

```
src/                  React 19 + Vite + Tailwind v4 + zustand
  api/client.ts       typed fetch, field-level errors
  offline/            IndexedDB cache, write queue, service worker registration
  store/              useTripStore (data + writes), useUiStore (where you are)
  hooks/              useDayGeometry (what to draw), useStopEditor (how stops change)
  components/         Spine, RouteMap, DayPanel, StopList, PlaceSearch, Editable
  views/              Route, Roadbook, Kit, Camps, Trip
  pages/              TripsPage (list), TripPage (/t/:token)
server/               Go 1.25, net/http, modernc.org/sqlite (no cgo)
  internal/trip/      document schema, validator, patch, day ops, JSON Schema
  internal/osrm/      road geometry;  internal/nominatim/  place lookup
  internal/store/     SQLite: trips, log, kit state, route cache
  internal/api/       routing, token access control, handlers
  internal/osrm/      rate-limited routing client
  seed/               the two built-in trips
prior_work/           the four standalone HTML/JSX files this grew out of
```

The trip is stored as a JSON blob rather than shredded into tables: it is edited as a
document, so export is a read and import is a write.

## The spine

The strip under the header is one segment per day, width proportional to the distance covered
— ridden or driven. Hatched green is the van, rose is riding, and logging actual km fills each
riding segment against its plan. Clicking a segment selects that day everywhere.

The two van bookends dwarf everything else. That is the argument behind the whole plan: run
the long drive first, then work back toward home.

## Testing

```bash
make test    # go test ./... and tsc --noEmit
```

## Gotchas

- **Never name a map component `Map`.** It shadows the global `Map` constructor. The component
  here is `RouteMap`.
- **A `<button>` with `display:flex` needs `items-stretch` spelled out** — the UA `align-items`
  collapses stretched children to zero width. That is why the spine bars carry it.
- **Two idb-keyval stores must not share a database name.** Each `createStore` opens the
  database at version 1 and creates only its own store, so the second one is silently missing
  and every write fails. `bike-trip-cache` and `bike-trip-queue` are separate for that reason.
- **OpenTopoMap, the OSRM demo server and Nominatim are courtesy services.** Route refresh
  walks days one at a time with a gap. Don't parallelise it. Nominatim's limit is a hard one
  request per second and it will block an anonymous client, which is why that call is
  server-side with a `User-Agent` and a query cache.
- **Day numbers are keys, not labels.** `log_entries.day` and `routes.day` both point at them.
  Never renumber `days[]` through a patch — `POST /trips/:token/days` moves the rows with it.
- **A Leaflet `useEffect` keyed on the day object refits the map on every edit.** Every patch
  produces a new object; `FitTo` keys on `day.n` and stops entirely while you are placing
  stops, or the camera fights the cursor.
- **This project uses pnpm.** There is a `pnpm-lock.yaml`; `npm install` produces a broken tree.

## Still open

Carried over from `prior_work/README.md` — all needing decisions or phone calls, not code:

- Campsites for the four Brittany bases: nothing researched
- Closing dates unconfirmed for every site, both plans
- Provence day 1: one 1 450 km push, or break it near Beaune and lose half a riding day
- A fuel-stop layer — the Finistère headlands and the Verdon both have long gaps
