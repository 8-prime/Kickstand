import type { LatLon } from "./geo";

/** Schematic fallback, used only when OSRM is unreachable.
 *
 *  The prior tools drew every route this way, via d3's
 *  `curveCatmullRom.alpha(0.6)`. This is the same curve sampled directly into
 *  lat/lon pairs, because Leaflet needs coordinates and not an SVG path.
 *
 *  These lines show which places a day links. They are not roads, and the app
 *  labels them as schematic wherever they appear. */
export function catmullRom(points: LatLon[], samplesPerSegment = 16, alpha = 0.6): LatLon[] {
  if (points.length < 2) return points.slice();

  // Duplicate the endpoints so the first and last segments have control points.
  const p = [points[0], ...points, points[points.length - 1]];
  const out: LatLon[] = [];

  for (let i = 1; i < p.length - 2; i++) {
    const p0 = p[i - 1];
    const p1 = p[i];
    const p2 = p[i + 1];
    const p3 = p[i + 2];

    const t0 = 0;
    const t1 = t0 + knot(p0, p1, alpha);
    const t2 = t1 + knot(p1, p2, alpha);
    const t3 = t2 + knot(p2, p3, alpha);

    for (let s = 0; s < samplesPerSegment; s++) {
      const t = t1 + ((t2 - t1) * s) / samplesPerSegment;
      out.push(interpolate(p0, p1, p2, p3, t0, t1, t2, t3, t));
    }
  }

  out.push(points[points.length - 1]);
  return out;
}

function knot(a: LatLon, b: LatLon, alpha: number): number {
  const d = Math.hypot(b[0] - a[0], b[1] - a[1]);
  // Coincident points would make the parametrisation blow up.
  return Math.max(Math.pow(d, alpha), 1e-6);
}

function interpolate(
  p0: LatLon,
  p1: LatLon,
  p2: LatLon,
  p3: LatLon,
  t0: number,
  t1: number,
  t2: number,
  t3: number,
  t: number,
): LatLon {
  const a1 = lerp(p0, p1, (t1 - t) / (t1 - t0), (t - t0) / (t1 - t0));
  const a2 = lerp(p1, p2, (t2 - t) / (t2 - t1), (t - t1) / (t2 - t1));
  const a3 = lerp(p2, p3, (t3 - t) / (t3 - t2), (t - t2) / (t3 - t2));
  const b1 = lerp(a1, a2, (t2 - t) / (t2 - t0), (t - t0) / (t2 - t0));
  const b2 = lerp(a2, a3, (t3 - t) / (t3 - t1), (t - t1) / (t3 - t1));
  return lerp(b1, b2, (t2 - t) / (t2 - t1), (t - t1) / (t2 - t1));
}

const lerp = (a: LatLon, b: LatLon, wa: number, wb: number): LatLon => [
  a[0] * wa + b[0] * wb,
  a[1] * wa + b[1] * wb,
];
