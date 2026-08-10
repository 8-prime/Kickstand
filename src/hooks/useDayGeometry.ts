import { useMemo } from "react";
import type { Day, TripPayload } from "../types";
import { catmullRom } from "../lib/catmull";
import { decodePolyline } from "../lib/polyline";
import { stopToLatLon, type LatLon } from "../lib/geo";
import { routeFor } from "../store/useTripStore";

export interface DayGeometry {
  /** The line to draw. Empty for a transfer day with no stops. */
  points: LatLon[];
  /** False means this is the schematic curve, not roads. */
  routed: boolean;
  /** Road distance through the stops, once routed. */
  routedKm: number | null;
  routedHours: number | null;
}

/** Resolves a day to a drawable line: real roads if the server has routed it,
 *  otherwise the schematic curve through the stops.
 *
 *  Routing itself happens on the server — one fetch serves everyone on the
 *  trip, and the geometry travels with the cached payload so it still draws
 *  with no signal. */
export function useDayGeometry(payload: TripPayload | null, day: Day): DayGeometry {
  const route = routeFor(payload, day.n);

  return useMemo(() => {
    const raw = day.stops.map(stopToLatLon);
    if (!raw.length) {
      return { points: [], routed: false, routedKm: null, routedHours: null };
    }

    if (route) {
      return {
        points: decodePolyline(route.polyline),
        routed: true,
        routedKm: route.km,
        routedHours: route.hours,
      };
    }

    return { points: catmullRom(raw), routed: false, routedKm: null, routedHours: null };
  }, [route, day.stops]);
}
