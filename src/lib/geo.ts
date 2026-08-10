import type { Stop } from "../types";

/** Leaflet's coordinate order. */
export type LatLon = [lat: number, lon: number];

export interface Bounds {
  lat0: number;
  lat1: number;
  lon0: number;
  lon1: number;
}

export const stopToLatLon = (s: Stop): LatLon => [s.lat, s.lon];

export function boundsOf(points: LatLon[]): Bounds | null {
  if (!points.length) return null;
  let lat0 = Infinity;
  let lat1 = -Infinity;
  let lon0 = Infinity;
  let lon1 = -Infinity;
  for (const [lat, lon] of points) {
    if (lat < lat0) lat0 = lat;
    if (lat > lat1) lat1 = lat;
    if (lon < lon0) lon0 = lon;
    if (lon > lon1) lon1 = lon;
  }
  return { lat0, lat1, lon0, lon1 };
}

/** Widen bounds so they contain every given point, or return null if they
 *  already do.
 *
 *  Only ever grows. A trip whose opening window is wider than its stops is a
 *  choice — the region you are riding in, not the box those stops happen to
 *  fit — and shrinking it back on every edit would quietly undo that.
 *
 *  Null rather than an unchanged copy so a caller can leave `bounds` out of the
 *  write entirely when nothing moved. */
export function growBounds(current: Bounds, points: LatLon[], pad = 0.15): Bounds | null {
  const box = boundsOf(points);
  if (!box) return null;

  const next: Bounds = {
    lat0: Math.min(current.lat0, box.lat0 - pad),
    lat1: Math.max(current.lat1, box.lat1 + pad),
    lon0: Math.min(current.lon0, box.lon0 - pad),
    lon1: Math.max(current.lon1, box.lon1 + pad),
  };

  const same =
    next.lat0 === current.lat0 &&
    next.lat1 === current.lat1 &&
    next.lon0 === current.lon0 &&
    next.lon1 === current.lon1;
  return same ? null : next;
}

/** A coordinate as a stop name, for when the geocoder has nothing to say. */
export const coordLabel = (lat: number, lon: number) =>
  `${lat.toFixed(4)}, ${lon.toFixed(4)}`;

/** Great-circle distance in km. Rough by design — it is used to tell whether
 *  two points are the same place, not to measure a ride. */
export function haversineKm(a: LatLon, b: LatLon): number {
  const R = 6371;
  const dLat = ((b[0] - a[0]) * Math.PI) / 180;
  const dLon = ((b[1] - a[1]) * Math.PI) / 180;
  const lat0 = (a[0] * Math.PI) / 180;
  const lat1 = (b[0] * Math.PI) / 180;
  const h =
    Math.sin(dLat / 2) ** 2 + Math.cos(lat0) * Math.cos(lat1) * Math.sin(dLon / 2) ** 2;
  return 2 * R * Math.asin(Math.sqrt(h));
}

export function pathLengthKm(points: LatLon[]): number {
  let total = 0;
  for (let i = 1; i < points.length; i++) total += haversineKm(points[i - 1], points[i]);
  return total;
}
