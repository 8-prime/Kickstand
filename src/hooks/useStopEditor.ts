import { useCallback, useMemo } from "react";
import type { Stop, TripDoc } from "../types";
import { growBounds, stopToLatLon } from "../lib/geo";
import { useTripStore } from "../store/useTripStore";

/** How long to wait after the last change before asking for road geometry.
 *  Long enough that dragging a stop across a region routes once, short enough
 *  that you see the real road while still looking at the day. */
const ROUTE_DELAY = 1200;

// Module scope, not component state: the panel that started an edit can be
// unmounted by the time the timer fires — selecting another day is enough —
// and the route still needs fetching.
const scheduled = new Map<number, ReturnType<typeof setTimeout>>();

function scheduleRoute(day: number) {
  const running = scheduled.get(day);
  if (running) clearTimeout(running);
  scheduled.set(
    day,
    setTimeout(() => {
      scheduled.delete(day);
      void useTripStore.getState().refreshRoutes([day]);
    }, ROUTE_DELAY),
  );
}

/** Everything that changes a day's stops, in one place.
 *
 *  Both the map and the stop list write through here so they cannot drift:
 *  every change is one patch of the whole `stops` array — what the server's
 *  patcher asks for, and what makes the offline queue collapse a burst of
 *  drags into the last one — plus the map bounds if a stop landed outside
 *  them, and a routing request once the dragging stops. */
export function useStopEditor(doc: TripDoc, index: number) {
  const patchMany = useTripStore((s) => s.patchMany);
  const day = doc.days[index];
  const stops = day?.stops ?? [];
  const dayN = day?.n ?? 0;
  const bounds = doc.bounds;

  const setStops = useCallback(
    async (next: Stop[]) => {
      const ops: { path: string; value: unknown }[] = [
        { path: `days[${index}].stops`, value: next },
      ];
      // A stop placed off the edge of the opening window widens it, in the
      // same revision. Otherwise the trip reopens somewhere that no longer
      // shows all of itself, and the validator starts warning about a stop
      // that is exactly where it was put.
      const wider = growBounds(bounds, next.map(stopToLatLon));
      if (wider) ops.push({ path: "bounds", value: wider });

      await patchMany(ops);
      if (next.length >= 2) scheduleRoute(dayN);
    },
    [bounds, dayN, index, patchMany],
  );

  return useMemo(
    () => ({
      stops,
      setStops,
      /** Append, or insert at a position. */
      add: (stop: Stop, at = stops.length) => {
        const next = [...stops];
        next.splice(Math.max(0, Math.min(at, stops.length)), 0, stop);
        return setStops(next);
      },
      remove: (i: number) => setStops(stops.filter((_, j) => j !== i)),
      moveAt: (from: number, to: number) => {
        if (from === to || from < 0 || from >= stops.length) return Promise.resolve();
        const next = [...stops];
        const [moved] = next.splice(from, 1);
        next.splice(Math.max(0, Math.min(to, next.length)), 0, moved);
        return setStops(next);
      },
      setCoords: (i: number, lat: number, lon: number) =>
        setStops(stops.map((s, j) => (j === i ? { ...s, lat, lon } : s))),
    }),
    [stops, setStops],
  );
}
