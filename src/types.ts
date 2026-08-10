/** These mirror server/internal/trip/schema.go. The server is the authority:
 *  it validates every document, so anything reaching the client has already
 *  been checked. Keep the two in step. */

/** A place a day passes through, in order.
 *
 *  An object rather than a [name, lat, lon] triple: the trip documents are
 *  written by hand and by language models, and a positional triple invites a
 *  swapped lat/lon that nothing catches but a map that looks wrong. */
export interface Stop {
  name: string;
  lat: number;
  lon: number;
}

export type DayType = "ride" | "van";

export interface Day {
  /** 1–N. Also the key the log and route cache use. */
  n: number;
  date?: string;
  type: DayType;
  title: string;
  /** Where you sleep, as written on the roadbook. */
  base?: string;
  detail?: string;
  /** Planned riding distance, km. 0 on a pure transfer day. */
  km: number;
  /** Van transfer distance, km. 0 on a riding day. */
  van: number;
  /** Planned saddle or driving time. */
  hours: number;
  stops: Stop[];
}

export interface Base {
  index: number;
  name: string;
  lat: number;
  lon: number;
  /** The day you arrive and pitch. */
  arriveDay: number;
  nights?: string;
}

export type CampsiteStatus = "picked" | "not-researched";

export interface Campsite {
  base: number;
  baseName?: string;
  status: CampsiteStatus;
  name?: string;
  rating?: number;
  reviews?: number;
  phone?: string;
  lat?: number;
  lon?: number;
  /** True when lat/lon are the base town rather than the pitch itself. */
  coordsApprox?: boolean;
  note?: string;
  closingDateVerified: boolean;
}

export interface KitItem {
  /** Stable across edits — tick state is keyed by it. */
  id: string;
  title: string;
  why?: string;
  /** A fine, or a thing still to confirm: "€135", "CHECK", "NEEDED". */
  flag?: string;
}

export interface KitGroup {
  group: string;
  /** Items here carry a fine rather than an inconvenience. */
  legal?: boolean;
  items: KitItem[];
}

export interface Bounds {
  lat0: number;
  lat1: number;
  lon0: number;
  lon1: number;
}

/** One whole trip. This is the document you export, edit and import. */
export interface TripDoc {
  schemaVersion: number;
  slug: string;
  name: string;
  subtitle?: string;
  origin?: string;
  dates?: string;
  bounds: Bounds;
  vanIn: number;
  vanOut: number;
  bases: Base[];
  days: Day[];
  campsites?: Campsite[];
  rejectedCampsites?: string[];
  campsiteCaveat?: string;
  kit?: KitGroup[];
}

/* ------------------------------ server state ----------------------------- */

/** Which log field an entry sets. */
export type LogField = "km" | "wx" | "note";

/** Weather mark index into WX / WX_NAME. */
export type WxIndex = 0 | 1 | 2 | 3;

export interface LogEntry {
  day: number;
  field: LogField;
  /** number for km, 0–3 for wx, string for note, null when cleared. */
  value: number | string | null;
  /** Client clock, epoch ms. Decides who wins a conflict. */
  updatedAt: number;
}

export interface KitEntry {
  itemId: string;
  checked: boolean;
  updatedAt: number;
}

export interface RouteGeometry {
  day: number;
  /** Encoded polyline, precision 5. */
  polyline: string;
  km: number;
  hours: number;
  signature: string;
  fetchedAt: string;
}

/** A geocoder result, in the shape a Stop wants. */
export interface Place {
  /** Short label, which becomes the stop name. */
  name: string;
  /** The full address, so two places of the same name are distinguishable. */
  displayName: string;
  lat: number;
  lon: number;
  kind?: string;
}

/** Adding, removing or moving a day. Not a patch: day numbers are keys the log
 *  and the route cache are stored against, so the server renumbers and remaps
 *  them together. */
export type DayOp =
  | { op: "insert"; after: number }
  | { op: "delete"; day: number }
  | { op: "move"; day: number; to: number };

export type Access = "view" | "edit";

/** Everything needed to render a trip and keep working with no signal. */
export interface TripPayload {
  id: string;
  slug: string;
  name: string;
  revision: number;
  access: Access;
  doc: TripDoc;
  log: LogEntry[];
  kit: KitEntry[];
  routes: RouteGeometry[];
  /** Present only for a caller holding the admin token. */
  viewToken?: string;
  editToken?: string;
  /** Things the write did that you did not ask for — a base moved to a
   *  different arrival day, say. The write happened; these are not errors. */
  warnings?: FieldError[];
}

export interface TripSummary {
  id: string;
  slug: string;
  name: string;
  subtitle?: string;
  dates?: string;
  days: number;
  updatedAt: string;
  viewToken: string;
  editToken: string;
}

/** One problem with a document, addressed by path: `days[4].stops[2].lon`. */
export interface FieldError {
  path: string;
  message: string;
}
