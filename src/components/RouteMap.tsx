import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import L from "leaflet";
import {
  MapContainer,
  Marker,
  Polyline,
  Popup,
  TileLayer,
  useMap,
  useMapEvents,
} from "react-leaflet";
import type { Base, Campsite, Day, TripDoc, TripPayload } from "../types";
import { api } from "../api/client";
import { useDayGeometry } from "../hooks/useDayGeometry";
import { useStopEditor } from "../hooks/useStopEditor";
import { boundsOf, coordLabel, growBounds, stopToLatLon, type LatLon } from "../lib/geo";
import { useTripStore } from "../store/useTripStore";
import { useUiStore, type BasemapId } from "../store/useUiStore";
import { Editable } from "./Editable";

/* Named RouteMap, never Map: a component called `Map` shadows the global Map
   constructor, which crashed an earlier version of this app. */

const BASEMAPS: Record<BasemapId, { url: string; attribution: string; label: string }> = {
  topo: {
    label: "Topo",
    url: "https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://opentopomap.org">OpenTopoMap</a> (CC-BY-SA) &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  },
  light: {
    label: "Light",
    url: "https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png",
    attribution:
      '&copy; <a href="https://carto.com/attributions">CARTO</a> &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
  },
  osm: {
    label: "Streets",
    url: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
  },
};

export function RouteMap({ doc, payload }: { doc: TripDoc; payload: TripPayload }) {
  const basemap = useUiStore((s) => s.basemap);
  const setBasemap = useUiStore((s) => s.setBasemap);
  const showCamps = useUiStore((s) => s.showCamps);
  const toggleCamps = useUiStore((s) => s.toggleCamps);
  const selectedDay = useUiStore((s) => s.selectedDay);
  const editing = useUiStore((s) => s.editing);
  const toggleEditing = useUiStore((s) => s.toggleEditing);

  const canEdit = payload.access === "edit";
  const dayIndex = selectedDay ? doc.days.findIndex((d) => d.n === selectedDay) : -1;
  const day = dayIndex >= 0 ? doc.days[dayIndex] : undefined;

  // Editing is about a day's stops, so it cannot outlive the selection that
  // says which day. Dropping back to the overview leaves the mode behind.
  const live = canEdit && editing;
  useEffect(() => {
    if (editing && !canEdit) useUiStore.getState().setEditing(false);
  }, [editing, canEdit]);

  const bounds = useMemo<L.LatLngBoundsExpression>(
    () => [
      [doc.bounds.lat0, doc.bounds.lon0],
      [doc.bounds.lat1, doc.bounds.lon1],
    ],
    [doc.bounds],
  );

  const vanPath = useMemo<LatLon[]>(
    () => doc.bases.map((b) => [b.lat, b.lon]),
    [doc.bases],
  );

  const camps = (doc.campsites ?? []).filter(
    (c) => c.status === "picked" && c.lat != null && c.lon != null,
  );

  return (
    <div className="relative">
      <MapContainer
        key={payload.id}
        bounds={bounds}
        scrollWheelZoom
        // Whole zoom steps overshoot badly when fitting a wide region into a
        // tall panel; fractional snapping gets the trip to fill the frame.
        zoomSnap={0.25}
        zoomDelta={0.5}
        className="h-[clamp(340px,58vh,620px)] w-full border border-paper-edge"
      >
        <TileLayer
          key={basemap}
          url={BASEMAPS[basemap].url}
          attribution={BASEMAPS[basemap].attribution}
          maxZoom={17}
        />

        <FitTo bounds={bounds} day={day} frozen={live} />

        {/* Where the van goes, base to base. */}
        <Polyline
          positions={vanPath}
          pathOptions={{
            color: "var(--color-transfer)",
            weight: 1.5,
            opacity: 0.55,
            dashArray: "2 7",
          }}
        />

        {doc.days
          .filter((d) => d.stops.length >= 2)
          .map((d) => (
            <DayLine
              key={d.n}
              payload={payload}
              day={d}
              dimmed={selectedDay != null && selectedDay !== d.n}
            />
          ))}

        {day && <Stops doc={doc} index={dayIndex} editing={live} />}

        {doc.bases.map((b, i) => (
          <BasePin
            key={b.index}
            base={b}
            index={i}
            doc={doc}
            editing={live}
            camp={(doc.campsites ?? []).find((c) => c.base === b.index)}
          />
        ))}

        {showCamps &&
          camps.map((c) => (
            <CampPin
              key={`camp-${c.base}`}
              camp={c}
              index={(doc.campsites ?? []).indexOf(c)}
              editing={live}
            />
          ))}
      </MapContainer>

      <div className="pointer-events-none absolute top-2 right-2 z-[1000] flex flex-col items-end gap-1">
        <div className="pointer-events-auto flex gap-px">
          {(Object.keys(BASEMAPS) as BasemapId[]).map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => setBasemap(id)}
              aria-pressed={basemap === id}
              className={[
                "border px-2 py-1 font-data text-[10px] tracking-wider uppercase",
                basemap === id
                  ? "border-ink bg-ink text-paper"
                  : "border-paper-edge bg-paper/90 text-ink-soft hover:text-ink",
              ].join(" ")}
            >
              {BASEMAPS[id].label}
            </button>
          ))}
        </div>
        {camps.length > 0 && (
          <button
            type="button"
            onClick={toggleCamps}
            aria-pressed={showCamps}
            className={[
              "pointer-events-auto border px-2 py-1 font-data text-[10px] tracking-wider uppercase",
              showCamps
                ? "border-ink bg-ink text-paper"
                : "border-paper-edge bg-paper/90 text-ink-soft hover:text-ink",
            ].join(" ")}
          >
            Camps
          </button>
        )}
        {canEdit && (
          <button
            type="button"
            onClick={toggleEditing}
            aria-pressed={editing}
            className={[
              "pointer-events-auto border px-2 py-1 font-data text-[10px] tracking-wider uppercase",
              editing
                ? "border-ride bg-ride text-paper"
                : "border-paper-edge bg-paper/90 text-ink-soft hover:text-ink",
            ].join(" ")}
          >
            {editing ? "Placing" : "Place"}
          </button>
        )}
      </div>

      {live && (
        <p
          role="status"
          className="pointer-events-none absolute bottom-2 left-2 z-[1000] max-w-[22rem] border-l-[3px] border-l-ride bg-paper/90 px-2 py-1 font-data text-[10px] leading-relaxed text-ink-soft"
        >
          {day
            ? `Day ${day.n}: drag a stop to move it, click the map to add one at the end, click a stop to rename or remove it. Base and campsite pins drag too.`
            : "Pick a day to place its stops. Base and campsite pins drag now."}
        </p>
      )}
    </div>
  );
}

/* --------------------------------- stops --------------------------------- */

/** The selected day's stops, and — in edit mode — the handles that move them. */
function Stops({
  doc,
  index,
  editing,
}: {
  doc: TripDoc;
  index: number;
  editing: boolean;
}) {
  const day = doc.days[index];
  const token = useTripStore((s) => s.token);
  const patch = useTripStore((s) => s.patch);
  const { stops, add, remove, setCoords } = useStopEditor(doc, index);

  // Where the stop being dragged is right now. Local, and only until the drag
  // ends: a patch per pointer move would be a document revision per pixel.
  const [drag, setDrag] = useState<{ i: number; pos: LatLon } | null>(null);

  const preview = useMemo<LatLon[]>(
    () =>
      drag
        ? stops.map((s, i) => (i === drag.i ? drag.pos : stopToLatLon(s)))
        : [],
    [drag, stops],
  );

  /** Name a stop that was just dropped on the map, if the geocoder knows it.
   *
   *  A second write rather than a slower first one: the stop appears under the
   *  cursor immediately with its coordinates as a name, and improves on itself
   *  a moment later. A geocoder that is down or rate limited costs a clumsy
   *  name, not a lost click. */
  const nameFromMap = useCallback(
    async (lat: number, lon: number) => {
      if (!token) return;
      try {
        const { place } = await api.reversePlace(token, lat, lon);
        if (!place.name) return;
        // Re-find the stop rather than trusting the index we appended at:
        // another edit may have landed while the lookup was in flight.
        const current =
          useTripStore.getState().payload?.doc.days[index]?.stops ?? [];
        const at = current.findIndex((s) => s.lat === lat && s.lon === lon);
        if (at >= 0) await patch(`days[${index}].stops[${at}].name`, place.name);
      } catch {
        /* the coordinates stand as the name */
      }
    },
    [index, patch, token],
  );

  const addAt = useCallback(
    async (lat: number, lon: number) => {
      await add({ name: coordLabel(lat, lon), lat, lon });
      void nameFromMap(lat, lon);
    },
    [add, nameFromMap],
  );

  return (
    <>
      {editing && <ClickToAdd onAdd={addAt} />}

      {drag && preview.length >= 2 && (
        <Polyline
          positions={preview}
          interactive={false}
          pathOptions={{
            color: day.type === "van" ? "var(--color-transfer)" : "var(--color-ride)",
            weight: 3,
            opacity: 0.7,
            dashArray: "3 6",
          }}
        />
      )}

      {stops.map((stop, i) => (
        <Marker
          key={`${day.n}-${i}`}
          position={drag?.i === i ? drag.pos : stopToLatLon(stop)}
          icon={editing ? stopIconEdit : stopIcon}
          keyboard={false}
          draggable={editing}
          eventHandlers={{
            drag: (e) => {
              const { lat, lng } = (e.target as L.Marker).getLatLng();
              setDrag({ i, pos: [lat, lng] });
            },
            dragend: (e) => {
              const { lat, lng } = (e.target as L.Marker).getLatLng();
              setDrag(null);
              void setCoords(i, round6(lat), round6(lng));
            },
          }}
        >
          <Popup>
            <b>{stop.name}</b>
            {editing && (
              <>
                <br />
                <span className="font-data text-[11px]">
                  <Editable
                    path={`days[${index}].stops[${i}].name`}
                    value={stop.name}
                    placeholder="Name this stop"
                  />
                </span>
                <br />
                <button
                  type="button"
                  onClick={() => void remove(i)}
                  className="mt-1 font-data text-[11px] text-alert underline underline-offset-2"
                >
                  Remove this stop
                </button>
              </>
            )}
          </Popup>
        </Marker>
      ))}
    </>
  );
}

/** Turns a click on empty map into a new stop at the end of the day. */
function ClickToAdd({ onAdd }: { onAdd: (lat: number, lon: number) => void }) {
  const last = useRef(0);

  useMapEvents({
    click: (e) => {
      // A double-click is two clicks as far as Leaflet is concerned, and
      // zooming in should not leave two stops behind where one was wanted.
      const now = Date.now();
      if (now - last.current < 350) return;
      last.current = now;
      onAdd(round6(e.latlng.lat), round6(e.latlng.lng));
    },
  });

  return null;
}

/* --------------------------------- lines --------------------------------- */

function DayLine({
  payload,
  day,
  dimmed,
}: {
  payload: TripPayload;
  day: Day;
  dimmed: boolean;
}) {
  const geo = useDayGeometry(payload, day);
  const selectDay = useUiStore((s) => s.selectDay);
  if (!geo.points.length) return null;

  const color = day.type === "van" ? "var(--color-transfer)" : "var(--color-ride)";

  return (
    <Polyline
      positions={geo.points}
      eventHandlers={{ click: () => selectDay(day.n) }}
      pathOptions={{
        color,
        weight: dimmed ? 2 : 4,
        opacity: dimmed ? 0.3 : 0.95,
        // Schematic lines are drawn broken, so they never read as real roads.
        dashArray: geo.routed ? undefined : "6 5",
        lineJoin: "round",
        lineCap: "round",
      }}
    >
      <Popup>
        <b>
          Day {day.n} · {day.title}
        </b>
        <br />
        {geo.routed
          ? `${geo.routedKm} km by road through these stops`
          : "Schematic line — not the road you will ride"}
      </Popup>
    </Polyline>
  );
}

/* --------------------------------- pins ---------------------------------- */

function BasePin({
  base,
  index,
  doc,
  camp,
  editing,
}: {
  base: Base;
  index: number;
  doc: TripDoc;
  camp?: Campsite;
  editing: boolean;
}) {
  const patchMany = useTripStore((s) => s.patchMany);

  return (
    <Marker
      position={[base.lat, base.lon]}
      icon={baseIcon(base.index)}
      zIndexOffset={500}
      draggable={editing}
      eventHandlers={{
        dragend: (e) => {
          const { lat, lng } = (e.target as L.Marker).getLatLng();
          void patchMany(
            withBounds(doc, [[lat, lng]], [
              { path: `bases[${index}].lat`, value: round6(lat) },
              { path: `bases[${index}].lon`, value: round6(lng) },
            ]),
          );
        },
      }}
    >
      <Popup>
        <b>
          Base {base.index} ·{" "}
          {editing ? (
            <Editable path={`bases[${index}].name`} value={base.name} />
          ) : (
            base.name
          )}
        </b>
        <br />
        {editing ? (
          <span className="font-data text-[11px]">
            <Editable
              path={`bases[${index}].nights`}
              value={base.nights}
              placeholder="How many nights?"
            />
            , arriving day{" "}
            <Editable
              path={`bases[${index}].arriveDay`}
              value={base.arriveDay}
              kind="number"
              className="w-12"
            />
          </span>
        ) : (
          <>
            {base.nights}, arriving day {base.arriveDay}
          </>
        )}
        {camp?.status === "picked" && (
          <>
            <br />
            {camp.name}
          </>
        )}
        {camp?.status === "not-researched" && (
          <>
            <br />
            <em>No campsite picked yet.</em>
          </>
        )}
      </Popup>
    </Marker>
  );
}

function CampPin({
  camp,
  index,
  editing,
}: {
  camp: Campsite;
  index: number;
  editing: boolean;
}) {
  const patchMany = useTripStore((s) => s.patchMany);

  return (
    <Marker
      position={[camp.lat!, camp.lon!]}
      icon={campIcon}
      draggable={editing}
      eventHandlers={{
        dragend: (e) => {
          const { lat, lng } = (e.target as L.Marker).getLatLng();
          void patchMany([
            { path: `campsites[${index}].lat`, value: round6(lat) },
            { path: `campsites[${index}].lon`, value: round6(lng) },
            // Dragging the pin off the base town and onto the pitch is exactly
            // the thing coordsApprox was waiting to stop being true.
            { path: `campsites[${index}].coordsApprox`, value: false },
          ]);
        },
      }}
    >
      <Popup>
        <b>{camp.name}</b>
        <br />
        {camp.rating}★ · {camp.reviews} reviews
        {camp.phone && (
          <>
            <br />
            <a href={`tel:${camp.phone.replace(/\s/g, "")}`}>{camp.phone}</a>
          </>
        )}
        {camp.coordsApprox && (
          <>
            <br />
            <em>
              Pin marks {camp.baseName}, not the pitch — the site is not geocoded.
              {editing && " Drag it onto the pitch to fix that."}
            </em>
          </>
        )}
      </Popup>
    </Marker>
  );
}

/* -------------------------------- camera --------------------------------- */

/** Pans and zooms to the selected day, or back out to the whole trip. */
function FitTo({
  bounds,
  day,
  frozen,
}: {
  bounds: L.LatLngBoundsExpression;
  day?: Day;
  frozen: boolean;
}) {
  const map = useMap();
  const reduceMotion =
    typeof window !== "undefined" &&
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

  // Keyed on the day number, not the day: every edit produces a new day object,
  // and refitting on each one would drag the camera away mid-edit.
  const dayN = day?.n;
  const hasStops = !!day?.stops.length;
  const target = day?.stops.length ? boundsOf(day.stops.map(stopToLatLon)) : null;
  const latest = useRef(target);
  latest.current = target;

  useEffect(() => {
    // While placing stops, the view is the thing being worked in. Moving it
    // under the cursor after every drop would make the map unusable.
    if (frozen) return;

    const box = latest.current;
    const next: L.LatLngBoundsExpression = box
      ? [
          [box.lat0, box.lon0],
          [box.lat1, box.lon1],
        ]
      : bounds;

    // Leaflet treats duration: 0 as "unset" and animates anyway, so reduced
    // motion has to take the non-flying path.
    if (reduceMotion) {
      map.fitBounds(next, { padding: [40, 40], maxZoom: 12, animate: false });
    } else {
      map.flyToBounds(next, { padding: [40, 40], duration: 0.7, maxZoom: 12 });
    }
  }, [map, dayN, hasStops, bounds, reduceMotion, frozen]);

  return null;
}

/* -------------------------------- helpers -------------------------------- */

/** Add a bounds op to a write when a point has landed outside the map window. */
function withBounds(
  doc: TripDoc,
  points: LatLon[],
  ops: { path: string; value: unknown }[],
) {
  const wider = growBounds(doc.bounds, points);
  return wider ? [...ops, { path: "bounds", value: wider }] : ops;
}

/** Six decimals is about 10 cm — past the point where more digits mean
 *  anything, and short enough that the stored document stays readable. */
const round6 = (v: number) => Math.round(v * 1e6) / 1e6;

const stopIcon = L.divIcon({
  className: "",
  html: '<div class="stop-dot"></div>',
  iconSize: [9, 9],
  iconAnchor: [4.5, 4.5],
});

// Bigger, and coloured, because in edit mode it is a handle to be grabbed
// rather than a mark to be read.
const stopIconEdit = L.divIcon({
  className: "",
  html: '<div class="stop-dot stop-dot--edit"></div>',
  iconSize: [17, 17],
  iconAnchor: [8.5, 8.5],
});

const campIcon = L.divIcon({
  className: "",
  html: '<div class="camp-pin">⌂</div>',
  iconSize: [20, 20],
  iconAnchor: [10, 10],
});

const baseIcon = (i: number) =>
  L.divIcon({
    className: "",
    html: `<div class="base-pin">${i}</div>`,
    iconSize: [26, 26],
    iconAnchor: [13, 13],
  });
