import { useMemo } from "react";
import type { Day, TripPayload } from "../types";
import { catmullRom } from "../lib/catmull";
import { decodePolyline } from "../lib/polyline";
import { haversineKm, stopToLatLon, type LatLon } from "../lib/geo";
import { routeFor } from "../store/useTripStore";

/** How far a routed line may start or end from the stop it was routed to
 *  before it stops being that day's route.
 *
 *  Not zero: the router snaps to the nearest road, which is legitimately a few
 *  hundred metres from a pin dropped on a hillside. Two kilometres is well
 *  clear of that and well inside "this stop has been moved". */
const SNAP_TOLERANCE_KM = 2;

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

    // The server keys its route cache by day number and only reroutes days
    // with two stops or more, so a day edited down to one would otherwise keep
    // drawing the road it used to have.
    if (route && raw.length >= 2) {
      const points = decodePolyline(route.polyline);
      if (points.length >= 2 && endpointsMatch(points, raw)) {
        return {
          points,
          routed: true,
          routedKm: route.km,
          routedHours: route.hours,
        };
      }
      // Routed from stops that have since moved. Fall through to the
      // schematic, which is drawn broken and says so — better than a real road
      // through somewhere this day no longer goes, right up until the refetch
      // lands a moment later.
    }

    return { points: catmullRom(raw), routed: false, routedKm: null, routedHours: null };
  }, [route, day.stops]);
}

/** Whether a routed line still begins and ends where the day's stops do.
 *
 *  A cheap stand-in for the server's stop-coordinate signature, which is a
 *  hash and cannot be recomputed here without the crypto API. It catches the
 *  edits that matter on screen — a stop dragged, added or removed at either
 *  end — and misses only a middle stop moving, which the refetch corrects
 *  within a second anyway. */
function endpointsMatch(points: LatLon[], stops: LatLon[]): boolean {
  return (
    haversineKm(points[0], stops[0]) <= SNAP_TOLERANCE_KM &&
    haversineKm(points[points.length - 1], stops[stops.length - 1]) <= SNAP_TOLERANCE_KM
  );
}
