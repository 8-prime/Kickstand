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

/** Great-circle distance in km. Used to sanity-check nothing, only to give the
 *  schematic fallback a rough length so the panel isn't blank. */
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
