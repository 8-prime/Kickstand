import { get, set, del, createStore } from "idb-keyval";
import type { TripPayload, TripSummary } from "../types";

/** IndexedDB rather than localStorage: a trip document plus its route
 *  polylines runs to a few hundred kilobytes, and several trips would sit
 *  uncomfortably close to the ~5 MB localStorage ceiling.
 *
 *  Its own database, not a second store inside a shared one: idb-keyval opens
 *  each database at version 1 and creates only the store it was asked for, so
 *  two createStore calls on one database name leave the second store missing
 *  and every write fails with NotFoundError. */
const db = createStore("bike-trip-cache", "trips");

const tripKey = (token: string) => `trip:${token}`;
const LIST_KEY = "trips";

export interface CachedTrip {
  payload: TripPayload;
  /** When this copy was taken, so the interface can say how stale it is. */
  cachedAt: number;
}

/** Store the last good copy of a trip, so the campsite with no signal still
 *  gets the map, the roadbook and the checklist. */
export async function cacheTrip(token: string, payload: TripPayload): Promise<void> {
  try {
    await set(tripKey(token), { payload, cachedAt: Date.now() } satisfies CachedTrip, db);
  } catch (err) {
    // A full or blocked database must not break the app — the network copy
    // is what is being displayed either way.
    console.warn("could not cache trip", err);
  }
}

export async function cachedTrip(token: string): Promise<CachedTrip | undefined> {
  try {
    return await get<CachedTrip>(tripKey(token), db);
  } catch {
    return undefined;
  }
}

export async function forgetTrip(token: string): Promise<void> {
  try {
    await del(tripKey(token), db);
  } catch {
    /* nothing worth reporting */
  }
}

export async function cacheTripList(trips: TripSummary[]): Promise<void> {
  try {
    await set(LIST_KEY, trips, db);
  } catch {
    /* nothing worth reporting */
  }
}

export async function cachedTripList(): Promise<TripSummary[] | undefined> {
  try {
    return await get<TripSummary[]>(LIST_KEY, db);
  } catch {
    return undefined;
  }
}
