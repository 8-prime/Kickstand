import { get, set, createStore } from "idb-keyval";
import type { KitEntry, LogEntry } from "../types";

/** Its own database — see the note in cache.ts on why two stores must not
 *  share one. */
const db = createStore("bike-trip-queue", "writes");
const KEY = "pending";

/** A write that has not reached the server yet.
 *
 *  Every variant is field-level and carries its own timestamp, so a queue
 *  flushed after an hour offline merges with whatever other people did in the
 *  meantime instead of overwriting it. That is the whole reason the API takes
 *  log entries and patch ops rather than whole documents. */
export type PendingWrite =
  | { kind: "log"; token: string; entry: LogEntry }
  | { kind: "kit"; token: string; entry: KitEntry }
  | { kind: "patch"; token: string; path: string; value: unknown; at: number };

type Listener = (count: number) => void;

let queue: PendingWrite[] = [];
let loaded = false;
const listeners = new Set<Listener>();

async function load(): Promise<void> {
  if (loaded) return;
  try {
    queue = (await get<PendingWrite[]>(KEY, db)) ?? [];
  } catch {
    queue = [];
  }
  loaded = true;
  notify();
}

async function persist(): Promise<void> {
  try {
    await set(KEY, queue, db);
  } catch (err) {
    console.warn("could not persist the write queue", err);
  }
  notify();
}

function notify() {
  for (const l of listeners) l(queue.length);
}

/** Subscribe to the pending count, for the connection indicator. */
export function onQueueChange(fn: Listener): () => void {
  listeners.add(fn);
  void load().then(() => fn(queue.length));
  return () => listeners.delete(fn);
}

export function pendingCount(): number {
  return queue.length;
}

/** Add a write to the queue.
 *
 *  Same-target writes collapse: editing a note five times while offline
 *  should flush as one entry, not five. The newest wins, which is also how
 *  the server settles them. */
export async function enqueue(write: PendingWrite): Promise<void> {
  await load();
  const key = targetOf(write);
  queue = queue.filter((w) => targetOf(w) !== key);
  queue.push(write);
  await persist();
}

function targetOf(w: PendingWrite): string {
  switch (w.kind) {
    case "log":
      return `log:${w.token}:${w.entry.day}:${w.entry.field}`;
    case "kit":
      return `kit:${w.token}:${w.entry.itemId}`;
    case "patch":
      return `patch:${w.token}:${w.path}`;
  }
}

export async function snapshot(): Promise<PendingWrite[]> {
  await load();
  return [...queue];
}

/** Drop the writes that were successfully sent, keeping anything added since
 *  the flush started. */
export async function forget(sent: PendingWrite[]): Promise<void> {
  await load();
  const done = new Set(sent.map(targetOf));
  queue = queue.filter((w) => !done.has(targetOf(w)));
  await persist();
}

export async function clear(): Promise<void> {
  queue = [];
  loaded = true;
  await persist();
}
