import type {
  DayOp,
  FieldError,
  KitEntry,
  LogEntry,
  Place,
  RouteGeometry,
  TripDoc,
  TripPayload,
  TripSummary,
} from "../types";

/** Same origin in production, and the Vite proxy forwards it in development. */
const BASE = "/api";

/** Where the admin token is kept. It is a shared secret for this server, not
 *  a per-person credential — see the README. */
const ADMIN_KEY = "bike-trip:admin-token";

export function adminToken(): string | null {
  return localStorage.getItem(ADMIN_KEY);
}

export function setAdminToken(token: string | null) {
  if (token) localStorage.setItem(ADMIN_KEY, token);
  else localStorage.removeItem(ADMIN_KEY);
}

/** An error the interface can act on: a message worth showing, the field
 *  problems behind it, and enough detail to recover from a conflict. */
export class ApiError extends Error {
  readonly status: number;
  readonly errors: FieldError[];
  readonly warnings: FieldError[];
  /** Set on 409: the revision the server is actually at. */
  readonly revision?: number;

  constructor(
    status: number,
    message: string,
    errors: FieldError[] = [],
    warnings: FieldError[] = [],
    revision?: number,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.errors = errors;
    this.warnings = warnings;
    this.revision = revision;
  }

  /** Someone else saved while you were editing. */
  get isConflict() {
    return this.status === 409;
  }

  /** The link is read-only, or the admin token is missing or wrong. */
  get isForbidden() {
    return this.status === 401 || this.status === 403;
  }

  /** No network. Distinguished from a server error because the fix differs:
   *  wait and retry, rather than change what you sent. */
  get isOffline() {
    return this.status === 0;
  }
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** Send the admin token, for the routes that need it. */
  admin?: boolean;
  /** Revision the caller believes it is editing, for optimistic concurrency. */
  ifMatch?: number;
  signal?: AbortSignal;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (opts.ifMatch !== undefined) headers["If-Match"] = String(opts.ifMatch);
  if (opts.admin) {
    const token = adminToken();
    if (token) headers["X-Admin-Token"] = token;
  }

  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: opts.signal,
    });
  } catch (err) {
    // fetch rejects with a bare TypeError when the network is down, and
    // "Failed to fetch" tells nobody anything.
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, "The server is unreachable.");
  }

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const body = text ? safeParse(text) : null;

  if (!res.ok) {
    throw new ApiError(
      res.status,
      body?.message ?? `The server returned ${res.status}.`,
      body?.errors ?? [],
      body?.warnings ?? [],
      body?.revision,
    );
  }

  return body as T;
}

function safeParse(text: string): any {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/* ------------------------------- endpoints ------------------------------- */

export const api = {
  /** Every trip on this server, with its links. Needs the admin token. */
  listTrips: () => request<TripSummary[]>("/trips", { admin: true }),

  /** Import a new trip. The response carries both share links, once. */
  createTrip: (doc: TripDoc) =>
    request<TripPayload>("/trips", { method: "POST", body: doc, admin: true }),

  deleteTrip: (token: string) =>
    request<void>(`/trips/${token}`, { method: "DELETE", admin: true }),

  rotateTokens: (token: string) =>
    request<{ viewToken: string; editToken: string }>(
      `/trips/${token}/tokens/rotate`,
      { method: "POST", admin: true },
    ),

  /** The whole trip: plan, log, checklist and cached routes in one payload —
   *  which is also exactly what gets stored for offline use. */
  getTrip: (token: string, signal?: AbortSignal) =>
    request<TripPayload>(`/trips/${token}`, { admin: true, signal }),

  /** Replace the document wholesale. Used by the import panel. */
  putTrip: (token: string, doc: TripDoc, ifMatch?: number) =>
    request<TripPayload>(`/trips/${token}`, {
      method: "PUT",
      body: doc,
      ifMatch,
      admin: true,
    }),

  /** Set individual fields. Used by the in-place editors, and by the offline
   *  queue, because field-level ops from two people merge and whole documents
   *  do not. */
  patchTrip: (token: string, ops: { path: string; value: unknown }[], ifMatch?: number) =>
    request<TripPayload>(`/trips/${token}`, {
      method: "PATCH",
      body: { ops },
      ifMatch,
      admin: true,
    }),

  putLog: (token: string, entries: LogEntry[]) =>
    request<{ log: LogEntry[] }>(`/trips/${token}/log`, {
      method: "PUT",
      body: { entries },
      admin: true,
    }),

  clearLog: (token: string) =>
    request<{ log: LogEntry[] }>(`/trips/${token}/log`, { method: "DELETE", admin: true }),

  putKit: (token: string, entries: KitEntry[]) =>
    request<{ kit: KitEntry[] }>(`/trips/${token}/kit`, {
      method: "PUT",
      body: { entries },
      admin: true,
    }),

  clearKit: (token: string) =>
    request<{ kit: KitEntry[] }>(`/trips/${token}/kit`, { method: "DELETE", admin: true }),

  /** Ask the server to fetch road geometry. Days omitted means everything
   *  whose stops have changed since it was last routed. */
  refreshRoutes: (token: string, days?: number[], force = false) =>
    request<{
      routes: RouteGeometry[];
      attempted: number;
      // Nullable: a server old enough to send a nil slice marshals it as null.
      failures: FieldError[] | null;
    }>(
      `/trips/${token}/routes/refresh`,
      { method: "POST", body: { days: days ?? [], force }, admin: true },
    ),

  /** Add, remove or move a day. Not a patch: the server renumbers the days and
   *  moves the log entries and cached routes keyed by those numbers with them,
   *  in one transaction. */
  dayOp: (token: string, op: DayOp, ifMatch?: number) =>
    request<TripPayload>(`/trips/${token}/days`, {
      method: "POST",
      body: op,
      ifMatch,
      admin: true,
    }),

  /** Look a place up by name. Server-side, because the OSM usage policy wants
   *  an identifying User-Agent and a browser cannot send one. */
  searchPlaces: (token: string, q: string, signal?: AbortSignal) =>
    request<{ places: Place[] }>(
      `/trips/${token}/places?q=${encodeURIComponent(q)}`,
      { admin: true, signal },
    ),

  /** Name the point a stop was just dropped on. */
  reversePlace: (token: string, lat: number, lon: number, signal?: AbortSignal) =>
    request<{ place: Place }>(
      `/trips/${token}/places/reverse?lat=${lat}&lon=${lon}`,
      { admin: true, signal },
    ),

  exportUrl: (token: string) => `${BASE}/trips/${token}/export`,
  schemaUrl: () => `${BASE}/schema/trip.json`,
  exampleUrl: () => `${BASE}/schema/example.json`,
};
