import type { LatLon } from "./geo";

/** Decoder for Google's encoded polyline format, which is what OSRM returns
 *  for `geometries=polyline`. Far more compact than GeoJSON — that matters
 *  because every route is cached in localStorage, which caps out around 5 MB.
 *
 *  Precision 5 is OSRM's default for `polyline`. */
export function decodePolyline(encoded: string, precision = 5): LatLon[] {
  const factor = Math.pow(10, precision);
  const points: LatLon[] = [];
  let index = 0;
  let lat = 0;
  let lon = 0;

  while (index < encoded.length) {
    let result = 0;
    let shift = 0;
    let byte: number;

    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);
    lat += result & 1 ? ~(result >> 1) : result >> 1;

    result = 0;
    shift = 0;
    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);
    lon += result & 1 ? ~(result >> 1) : result >> 1;

    points.push([lat / factor, lon / factor]);
  }

  return points;
}
