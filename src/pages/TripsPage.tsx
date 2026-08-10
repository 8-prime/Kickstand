import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import type { FieldError, TripSummary } from "../types";
import { ApiError, adminToken, api, setAdminToken } from "../api/client";
import { cacheTripList, cachedTripList } from "../offline/cache";
import { km as fmtKm } from "../lib/format";

/** Every trip on this server.
 *
 *  Gated behind the admin token, because the listing carries the share links:
 *  without a gate the links would protect nothing, since anyone who could
 *  reach the server could read them all. */
export function TripsPage() {
  const [token, setToken] = useState(adminToken() ?? "");
  const [trips, setTrips] = useState<TripSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [needsToken, setNeedsToken] = useState(!adminToken());

  const load = async () => {
    setError(null);
    try {
      const list = await api.listTrips();
      setTrips(list);
      setNeedsToken(false);
      await cacheTripList(list);
    } catch (err) {
      if (err instanceof ApiError && err.isForbidden) {
        setNeedsToken(true);
        setError(err.message);
        return;
      }
      const cached = await cachedTripList();
      if (cached) {
        setTrips(cached);
        setError("Offline — showing the list from the last time you were connected.");
        return;
      }
      setError(err instanceof Error ? err.message : "Could not reach the server.");
    }
  };

  useEffect(() => {
    if (adminToken()) void load();
  }, []);

  return (
    <div className="mx-auto max-w-4xl px-4 pt-10 pb-16 sm:px-6">
      <p className="eyebrow">Trip planner</p>
      <h1 className="mt-1 text-[clamp(1.75rem,5vw,2.75rem)]">
        Every trip on
        <br className="hidden sm:block" /> this server.
      </h1>

      {needsToken ? (
        <section className="panel mt-6 p-4">
          <p className="eyebrow">Admin token</p>
          <p className="mt-1.5 text-[14.5px] text-ink-soft">
            The trip list carries the share links for every trip, so it needs the server's
            admin token. It is printed in the server log at startup, or set by you in{" "}
            <code>BIKETRIP_ADMIN_TOKEN</code>. It is kept in this browser afterwards.
          </p>
          <form
            className="mt-3 flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              setAdminToken(token.trim() || null);
              void load();
            }}
          >
            <input
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="admin token"
              className="field min-w-[260px] flex-1"
              autoComplete="off"
            />
            <button
              type="submit"
              className="border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper"
            >
              Use it
            </button>
          </form>
          {error && <p className="mt-2 font-data text-[11px] text-alert">{error}</p>}
          <p className="mt-3 font-data text-[10.5px] leading-relaxed text-ink-soft">
            If you were sent a link to one trip, you do not need this — open the link.
          </p>
        </section>
      ) : (
        <>
          {error && <p className="mt-4 font-data text-[11px] text-alert">{error}</p>}

          <ul className="mt-6 space-y-2">
            {(trips ?? []).map((t) => (
              <li key={t.id}>
                <Link
                  to={`/t/${t.editToken}`}
                  className="panel flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1 p-4 transition-colors hover:border-ink"
                >
                  <span>
                    <span className="block font-data text-[9px] tracking-[0.18em] text-ink-soft uppercase">
                      {t.subtitle || t.slug}
                    </span>
                    <span className="mt-0.5 block font-display text-xl font-extrabold tracking-tight">
                      {t.name}
                    </span>
                  </span>
                  <span className="font-data text-[11px] text-ink-soft">
                    {fmtKm(t.days)} days {t.dates ? `· ${t.dates}` : ""}
                  </span>
                </Link>
              </li>
            ))}
          </ul>

          {trips && trips.length === 0 && (
            <p className="mt-6 text-[14.5px] text-ink-soft">
              No trips yet. Import one with <code>POST /api/trips</code>, or start the server
              without <code>-no-seed</code> to get the two built-in plans.
            </p>
          )}

          <NewTrip onCreated={load} />

          <button
            type="button"
            onClick={() => {
              setAdminToken(null);
              setNeedsToken(true);
              setTrips(null);
            }}
            className="mt-8 font-data text-[10px] tracking-[0.12em] text-ink-soft uppercase underline underline-offset-2 hover:text-ink"
          >
            Forget the admin token on this browser
          </button>
        </>
      )}
    </div>
  );
}

function NewTrip({ onCreated }: { onCreated: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<FieldError[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const create = async (raw: string) => {
    setErrors([]);
    setMessage(null);

    let doc;
    try {
      doc = JSON.parse(raw);
    } catch (err) {
      setErrors([{ path: "", message: err instanceof Error ? err.message : "Invalid JSON" }]);
      return;
    }

    setBusy(true);
    try {
      const created = await api.createTrip(doc);
      setText("");
      setOpen(false);
      setMessage(`Added ${created.name}.`);
      await onCreated();
    } catch (err) {
      if (err instanceof ApiError) {
        setMessage(err.message);
        setErrors(err.errors);
      } else {
        setMessage("Could not add that trip.");
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="mt-6">
      {!open ? (
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper"
        >
          Add a trip
        </button>
      ) : (
        <div className="panel p-4">
          <p className="eyebrow">New trip</p>
          <p className="mt-1.5 text-[14.5px] text-ink-soft">
            Paste a trip document, or upload one. The format is published at{" "}
            <a href={api.schemaUrl()} target="_blank" rel="noreferrer" className="underline">
              /api/schema/trip.json
            </a>{" "}
            with a worked example at{" "}
            <a href={api.exampleUrl()} target="_blank" rel="noreferrer" className="underline">
              /api/schema/example.json
            </a>{" "}
            — enough for a model to write you a new one.
          </p>

          <label className="mt-3 inline-block cursor-pointer border border-paper-edge px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase hover:border-ink">
            Upload a file
            <input
              type="file"
              accept="application/json,.json"
              className="sr-only"
              onChange={async (e) => {
                const f = e.target.files?.[0];
                if (f) await create(await f.text());
                e.target.value = "";
              }}
            />
          </label>

          <textarea
            rows={8}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="…or paste the trip JSON here"
            spellCheck={false}
            className="field mt-3 w-full resize-y font-data text-[12px] leading-relaxed"
          />
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              disabled={busy || !text.trim()}
              onClick={() => void create(text)}
              className="border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper disabled:opacity-40"
            >
              {busy ? "Checking…" : "Add it"}
            </button>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="border border-paper-edge px-3 py-1.5 font-data text-[11px] tracking-[0.12em] text-ink-soft uppercase hover:border-ink"
            >
              Cancel
            </button>
          </div>

          {errors.length > 0 && (
            <ul className="mt-3 space-y-1">
              {errors.map((e, i) => (
                <li key={i} className="font-data text-[11.5px]">
                  <span className="text-alert">{e.path || "document"}</span>
                  <span className="text-ink-soft"> — {e.message}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {message && <p className="mt-2 font-data text-[11px] text-ink-soft">{message}</p>}
    </section>
  );
}
