import { useEffect, useMemo } from "react";
import L from "leaflet";
import { MapContainer, Marker, Polyline, Popup, TileLayer, useMap } from "react-leaflet";
import type { Base, Campsite, Day, TripDoc, TripPayload } from "../types";
import { useDayGeometry } from "../hooks/useDayGeometry";
import { boundsOf, stopToLatLon, type LatLon } from "../lib/geo";
import { useUiStore, type BasemapId } from "../store/useUiStore";

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

  const day = selectedDay ? doc.days.find((d) => d.n === selectedDay) : undefined;

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

        <FitTo bounds={bounds} day={day} />

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

        {day?.stops.map((s, i) => (
          <Marker
            key={`${day.n}-${i}-${s.name}`}
            position={stopToLatLon(s)}
            icon={stopIcon}
            keyboard={false}
          >
            <Popup>
              <b>{s.name}</b>
            </Popup>
          </Marker>
        ))}

        {doc.bases.map((b) => (
          <BasePin
            key={b.index}
            base={b}
            camp={(doc.campsites ?? []).find((c) => c.base === b.index)}
          />
        ))}

        {showCamps &&
          camps.map((c) => (
            <Marker key={`camp-${c.base}`} position={[c.lat!, c.lon!]} icon={campIcon}>
              <Popup>
                <b>{c.name}</b>
                <br />
                {c.rating}★ · {c.reviews} reviews
                {c.phone && (
                  <>
                    <br />
                    <a href={`tel:${c.phone.replace(/\s/g, "")}`}>{c.phone}</a>
                  </>
                )}
                {c.coordsApprox && (
                  <>
                    <br />
                    <em>Pin marks {c.baseName}, not the pitch — the site is not geocoded.</em>
                  </>
                )}
              </Popup>
            </Marker>
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
      </div>
    </div>
  );
}

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

function BasePin({ base, camp }: { base: Base; camp?: Campsite }) {
  return (
    <Marker position={[base.lat, base.lon]} icon={baseIcon(base.index)} zIndexOffset={500}>
      <Popup>
        <b>
          Base {base.index} · {base.name}
        </b>
        <br />
        {base.nights}, arriving day {base.arriveDay}
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

/** Pans and zooms to the selected day, or back out to the whole trip. */
function FitTo({ bounds, day }: { bounds: L.LatLngBoundsExpression; day?: Day }) {
  const map = useMap();
  const reduceMotion =
    typeof window !== "undefined" &&
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

  useEffect(() => {
    const target = day?.stops.length ? boundsOf(day.stops.map(stopToLatLon)) : null;

    const next: L.LatLngBoundsExpression = target
      ? [
          [target.lat0, target.lon0],
          [target.lat1, target.lon1],
        ]
      : bounds;

    // Leaflet treats duration: 0 as "unset" and animates anyway, so reduced
    // motion has to take the non-flying path.
    if (reduceMotion) {
      map.fitBounds(next, { padding: [40, 40], maxZoom: 12, animate: false });
    } else {
      map.flyToBounds(next, { padding: [40, 40], duration: 0.7, maxZoom: 12 });
    }
  }, [map, day, bounds, reduceMotion]);

  return null;
}

const stopIcon = L.divIcon({
  className: "",
  html: '<div class="stop-dot"></div>',
  iconSize: [9, 9],
  iconAnchor: [4.5, 4.5],
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
