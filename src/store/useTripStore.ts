import { create } from "zustand";
import { ApiError, api } from "../api/client";
import { cacheTrip, cachedTrip } from "../offline/cache";
import { enqueue, forget, onQueueChange, snapshot, type PendingWrite } from "../offline/queue";
import type {
  DayOp,
  KitEntry,
  LogEntry,
  LogField,
  TripDoc,
  TripPayload,
  WxIndex,
} from "../types";

type Status = "idle" | "loading" | "ready" | "error";

interface TripState {
  token: string | null;
  payload: TripPayload | null;
  status: Status;
  error: string | null;
  /** Something the server did that you did not ask for and should know about.
   *  Not a failure — the write landed. */
  notice: string | null;

  /** True when the trip on screen came out of the cache. */
  fromCache: boolean;
  cachedAt: number | null;
  online: boolean;
  pending: number;
  /** Non-null while a route refresh is running. */
  routing: { done: number; total: number } | null;

  load: (token: string) => Promise<void>;
  reload: () => Promise<void>;

  setLog: (day: number, field: LogField, value: number | string | null) => Promise<void>;
  setWx: (day: number, wx: WxIndex) => Promise<void>;
  toggleKit: (itemId: string) => Promise<void>;
  clearLog: () => Promise<void>;
  clearKit: () => Promise<void>;

  /** Set one field of the trip document — what the in-place editors call. */
  patch: (path: string, value: unknown) => Promise<void>;
  /** Set several fields as one revision. Dragging a stop moves its coordinates
   *  and can widen the map bounds; those belong in one write, not three. */
  patchMany: (ops: { path: string; value: unknown }[]) => Promise<void>;
  /** Replace the whole document — what the import panel calls. */
  replaceDoc: (doc: TripDoc) => Promise<void>;

  /** Add, remove or move a day. Online only: it renumbers the whole trip. */
  dayOp: (op: DayOp) => Promise<void>;

  refreshRoutes: (days?: number[], force?: boolean) => Promise<void>;
  flush: () => Promise<void>;
  dismissNotice: () => void;
}

export const useTripStore = create<TripState>((set, get) => ({
  token: null,
  payload: null,
  status: "idle",
  error: null,
  notice: null,
  fromCache: false,
  cachedAt: null,
  online: typeof navigator === "undefined" ? true : navigator.onLine,
  pending: 0,
  routing: null,

  load: async (token) => {
    set({ token, status: "loading", error: null });

    // Show the cached copy first if there is one. On a bad connection this is
    // the difference between a usable app and a spinner.
    const cached = await cachedTrip(token);
    if (cached) {
      set({
        payload: applyPending(cached.payload, await snapshot(), token),
        status: "ready",
        fromCache: true,
        cachedAt: cached.cachedAt,
      });
    }

    try {
      const payload = await api.getTrip(token);
      await cacheTrip(token, payload);
      set({
        payload: applyPending(payload, await snapshot(), token),
        status: "ready",
        fromCache: false,
        cachedAt: null,
        error: null,
      });
      await get().flush();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Could not load that trip.";
      // A cached copy plus no network is a working app, not an error.
      if (get().payload) set({ error: message });
      else set({ status: "error", error: message });
    }
  },

  reload: async () => {
    const token = get().token;
    if (token) await get().load(token);
  },

  setLog: async (day, field, value) => {
    const entry: LogEntry = { day, field, value, updatedAt: Date.now() };
    set((s) => ({ payload: withLog(s.payload, entry) }));
    await send(get, set, { kind: "log", token: get().token!, entry });
  },

  setWx: async (day, wx) => {
    const current = logValue(get().payload, day, "wx");
    // Tapping the active mark clears it.
    await get().setLog(day, "wx", current === wx ? null : wx);
  },

  toggleKit: async (itemId) => {
    const checked = !kitChecked(get().payload, itemId);
    const entry: KitEntry = { itemId, checked, updatedAt: Date.now() };
    set((s) => ({ payload: withKit(s.payload, entry) }));
    await send(get, set, { kind: "kit", token: get().token!, entry });
  },

  clearLog: async () => {
    const token = get().token;
    if (!token) return;
    try {
      const { log } = await api.clearLog(token);
      set((s) => (s.payload ? { payload: { ...s.payload, log } } : {}));
      await cacheCurrent(get);
    } catch (err) {
      set({ error: messageOf(err) });
    }
  },

  clearKit: async () => {
    const token = get().token;
    if (!token) return;
    try {
      const { kit } = await api.clearKit(token);
      set((s) => (s.payload ? { payload: { ...s.payload, kit } } : {}));
      await cacheCurrent(get);
    } catch (err) {
      set({ error: messageOf(err) });
    }
  },

  patch: async (path, value) => get().patchMany([{ path, value }]),

  patchMany: async (ops) => {
    const token = get().token;
    const payload = get().payload;
    if (!token || !payload || !ops.length) return;

    // Optimistic: the editor closes on the value you typed, not on a round trip.
    const doc = ops.reduce((d, op) => setIn(d, op.path, op.value), payload.doc);
    set({ payload: { ...payload, doc } });

    if (!get().online) {
      // Queued one op at a time, not as a batch: the queue collapses repeated
      // writes to the same path, and a batch would hide them from that.
      for (const op of ops) {
        await enqueue({ kind: "patch", token, path: op.path, value: op.value, at: Date.now() });
      }
      set({ pending: (await snapshot()).length });
      await cacheCurrent(get);
      return;
    }

    try {
      const updated = await api.patchTrip(token, ops, payload.revision);
      set({ payload: updated, error: null });
      await cacheTrip(token, updated);
    } catch (err) {
      if (err instanceof ApiError && err.isConflict) {
        // Someone else saved. Take their version, then reapply these edits.
        await get().reload();
        try {
          const retried = await api.patchTrip(token, ops);
          set({ payload: retried, error: null });
          await cacheTrip(token, retried);
          return;
        } catch (retryErr) {
          set({ error: messageOf(retryErr) });
          return;
        }
      }
      if (err instanceof ApiError && err.isOffline) {
        for (const op of ops) {
          await enqueue({ kind: "patch", token, path: op.path, value: op.value, at: Date.now() });
        }
        set({ pending: (await snapshot()).length });
        return;
      }
      // Rejected on its merits: put the old value back rather than leaving a
      // change on screen that the server does not have.
      set({ payload, error: messageOf(err) });
    }
  },

  replaceDoc: async (doc) => {
    const token = get().token;
    const payload = get().payload;
    if (!token) return;

    const updated = await api.putTrip(token, doc, payload?.revision);
    set({ payload: updated, error: null });
    await cacheTrip(token, updated);
  },

  dayOp: async (op) => {
    const token = get().token;
    const payload = get().payload;
    if (!token || !payload) return;

    // Not queueable, deliberately. Every other edit names a field, so two
    // people editing offline merge; this one renumbers the days that naming
    // depends on, and there is no honest way to replay it against someone
    // else's version later.
    if (!get().online) {
      set({ error: "Adding, removing or moving a day needs a connection — it renumbers the trip." });
      return;
    }

    try {
      const updated = await api.dayOp(token, op, payload.revision);
      set({
        payload: updated,
        error: null,
        notice: updated.warnings?.length
          ? updated.warnings.map((w) => w.message).join(" ")
          : null,
      });
      await cacheTrip(token, updated);
    } catch (err) {
      if (err instanceof ApiError && err.isConflict) {
        await get().reload();
        set({ error: "Someone else saved first. You have their version now — try that again." });
        return;
      }
      set({ error: messageOf(err) });
    }
  },

  dismissNotice: () => set({ notice: null }),

  refreshRoutes: async (days, force) => {
    const token = get().token;
    if (!token) return;

    set({ routing: { done: 0, total: days?.length ?? 0 } });
    try {
      const { routes, failures } = await api.refreshRoutes(token, days, force);
      set((s) => ({
        payload: s.payload ? { ...s.payload, routes } : s.payload,
        error: failures.length ? failures[0].message : null,
      }));
      await cacheCurrent(get);
    } catch (err) {
      set({ error: messageOf(err) });
    } finally {
      set({ routing: null });
    }
  },

  flush: async () => {
    const token = get().token;
    if (!token || !get().online) return;

    const pending = (await snapshot()).filter((w) => w.token === token);
    if (!pending.length) return;

    const sent: PendingWrite[] = [];

    const logs = pending.filter((w) => w.kind === "log").map((w) => w.entry);
    if (logs.length) {
      try {
        await api.putLog(token, logs);
        sent.push(...pending.filter((w) => w.kind === "log"));
      } catch {
        /* still offline; try again next time */
      }
    }

    const kits = pending.filter((w) => w.kind === "kit").map((w) => w.entry);
    if (kits.length) {
      try {
        await api.putKit(token, kits);
        sent.push(...pending.filter((w) => w.kind === "kit"));
      } catch {
        /* leave queued */
      }
    }

    const patches = pending.filter((w) => w.kind === "patch");
    if (patches.length) {
      try {
        // Field-level ops in one request: two people who edited different
        // fields while offline both survive.
        await api.patchTrip(
          token,
          patches.map((p) => ({ path: p.path, value: p.value })),
        );
        sent.push(...patches);
      } catch (err) {
        // A patch the server refuses would block the queue forever, so drop
        // it and say why.
        if (err instanceof ApiError && !err.isOffline) {
          sent.push(...patches);
          set({ error: `Some offline edits could not be applied: ${err.message}` });
        }
      }
    }

    if (sent.length) {
      await forget(sent);
      set({ pending: (await snapshot()).length });
      // Re-read so everyone's merged result is what is on screen. This is
      // also live data again, so the "showing a saved copy" notice must go —
      // leaving it up would keep warning about staleness that is over.
      try {
        const fresh = await api.getTrip(token);
        set({ payload: fresh, fromCache: false, cachedAt: null, error: null });
        await cacheTrip(token, fresh);
      } catch {
        /* the optimistic state stands */
      }
    }
  },
}));

/* ------------------------------- plumbing -------------------------------- */

/** Queue a write, or send it now. Either way the screen already shows it. */
async function send(
  get: () => TripState,
  set: (partial: Partial<TripState>) => void,
  write: Extract<PendingWrite, { kind: "log" | "kit" }>,
) {
  const token = write.token;
  if (!token) return;

  if (!get().online) {
    await enqueue(write);
    set({ pending: (await snapshot()).length });
    await cacheCurrent(get);
    return;
  }

  try {
    if (write.kind === "log") {
      const { log } = await api.putLog(token, [write.entry]);
      const payload = get().payload;
      if (payload) set({ payload: { ...payload, log } });
    } else {
      const { kit } = await api.putKit(token, [write.entry]);
      const payload = get().payload;
      if (payload) set({ payload: { ...payload, kit } });
    }
    set({ error: null });
    await cacheCurrent(get);
  } catch (err) {
    if (err instanceof ApiError && err.isOffline) {
      await enqueue(write);
      set({ pending: (await snapshot()).length });
      return;
    }
    set({ error: messageOf(err) });
  }
}

async function cacheCurrent(get: () => TripState) {
  const { token, payload } = get();
  if (token && payload) await cacheTrip(token, payload);
}

/** Fold queued writes onto a payload, so a cached trip shows what you logged
 *  offline rather than the last thing the server knew about. */
function applyPending(payload: TripPayload, pending: PendingWrite[], token: string): TripPayload {
  let out = payload;
  for (const w of pending) {
    if (w.token !== token) continue;
    if (w.kind === "log") out = withLog(out, w.entry)!;
    else if (w.kind === "kit") out = withKit(out, w.entry)!;
    else out = { ...out, doc: setIn(out.doc, w.path, w.value) };
  }
  return out;
}

function withLog(payload: TripPayload | null, entry: LogEntry): TripPayload | null {
  if (!payload) return payload;
  const log = payload.log.filter((e) => !(e.day === entry.day && e.field === entry.field));
  if (entry.value !== null) log.push(entry);
  return { ...payload, log };
}

function withKit(payload: TripPayload | null, entry: KitEntry): TripPayload | null {
  if (!payload) return payload;
  const kit = payload.kit.filter((e) => e.itemId !== entry.itemId);
  kit.push(entry);
  return { ...payload, kit };
}

/** Set a value at `days[4].km`, immutably. Mirrors ApplyPatch on the server so
 *  the optimistic update and the stored result agree. */
function setIn<T>(root: T, path: string, value: unknown): T {
  const segs = parsePath(path);
  if (!segs.length) return root;

  const clone = (node: any): any => (Array.isArray(node) ? [...node] : { ...node });

  const out = clone(root);
  let cursor: any = out;
  for (let i = 0; i < segs.length - 1; i++) {
    const seg = segs[i];
    const next = clone(cursor[seg]);
    cursor[seg] = next;
    cursor = next;
  }
  cursor[segs[segs.length - 1]] = value;
  return out;
}

function parsePath(path: string): (string | number)[] {
  const out: (string | number)[] = [];
  for (const part of path.split(".")) {
    let name = part;
    let open = name.indexOf("[");
    while (open >= 0) {
      const close = name.indexOf("]", open);
      if (close < 0) break;
      if (open > 0) out.push(name.slice(0, open));
      out.push(Number(name.slice(open + 1, close)));
      name = name.slice(close + 1);
      open = name.indexOf("[");
    }
    if (name) out.push(name);
  }
  return out;
}

const messageOf = (err: unknown) =>
  err instanceof Error ? err.message : "Something went wrong.";

/* ------------------------------- selectors ------------------------------- */

export function logValue(
  payload: TripPayload | null,
  day: number,
  field: LogField,
): number | string | null {
  const entry = payload?.log.find((e) => e.day === day && e.field === field);
  return entry?.value ?? null;
}

export function kitChecked(payload: TripPayload | null, itemId: string): boolean {
  return payload?.kit.find((e) => e.itemId === itemId)?.checked ?? false;
}

export function routeFor(payload: TripPayload | null, day: number) {
  return payload?.routes.find((r) => r.day === day);
}

export function loggedKm(payload: TripPayload | null): number {
  return (payload?.log ?? [])
    .filter((e) => e.field === "km")
    .reduce((sum, e) => sum + (Number(e.value) || 0), 0);
}

export function loggedDays(payload: TripPayload | null): number {
  return (payload?.log ?? []).filter((e) => e.field === "km" && Number(e.value) > 0).length;
}

/* --------------------------- connection tracking -------------------------- */

if (typeof window !== "undefined") {
  const setOnline = (online: boolean) => {
    useTripStore.setState({ online });
    if (!online) return;
    // Push what was queued, then take a fresh copy. The reload is what
    // retires the "server is unreachable" notice and the cached-copy warning
    // — without it they would sit there long after the signal came back.
    void useTripStore
      .getState()
      .flush()
      .then(() => useTripStore.getState().reload());
  };
  window.addEventListener("online", () => setOnline(true));
  window.addEventListener("offline", () => setOnline(false));
  onQueueChange((count) => useTripStore.setState({ pending: count }));
}
