import { useEffect, useRef, useState } from "react";
import type { Place } from "../types";
import { ApiError, api } from "../api/client";
import { useTripStore } from "../store/useTripStore";

/** Type a place name, get coordinates.
 *
 *  The lookup runs on the server, which is where the rate limiting and the
 *  identifying User-Agent the OSM geocoder asks for both live. This side's job
 *  is to not ask more often than a person is actually typing. */
export function PlaceSearch({
  onPick,
  placeholder = "Search a place to add…",
  label,
}: {
  onPick: (place: Place) => void;
  placeholder?: string;
  label?: string;
}) {
  const token = useTripStore((s) => s.token);
  const online = useTripStore((s) => s.online);

  const [query, setQuery] = useState("");
  const [places, setPlaces] = useState<Place[]>([]);
  const [status, setStatus] = useState<"idle" | "searching" | "empty" | "error">("idle");
  const [message, setMessage] = useState("");
  const abort = useRef<AbortController | null>(null);

  useEffect(() => {
    const q = query.trim();
    if (!token || q.length < 2) {
      setPlaces([]);
      setStatus("idle");
      return;
    }
    if (!online) {
      setStatus("error");
      setMessage("Place search needs a connection. Drop a stop on the map instead.");
      return;
    }

    // 400 ms after the last keystroke, and the previous request is abandoned
    // rather than raced: results arriving out of order would show answers to a
    // question that is no longer on screen.
    const timer = setTimeout(() => {
      abort.current?.abort();
      const controller = new AbortController();
      abort.current = controller;

      setStatus("searching");
      api
        .searchPlaces(token, q, controller.signal)
        .then(({ places }) => {
          setPlaces(places);
          setStatus(places.length ? "idle" : "empty");
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setPlaces([]);
          setStatus("error");
          setMessage(
            err instanceof ApiError ? err.message : "That search did not come back.",
          );
        });
    }, 400);

    return () => clearTimeout(timer);
  }, [query, token, online]);

  useEffect(() => () => abort.current?.abort(), []);

  const pick = (place: Place) => {
    onPick(place);
    setQuery("");
    setPlaces([]);
    setStatus("idle");
  };

  return (
    <div className="mt-2">
      {label && <p className="eyebrow mb-1">{label}</p>}
      <input
        type="search"
        value={query}
        placeholder={placeholder}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setQuery("");
          }
          // Enter with one obvious answer takes it, so a confident search is a
          // type-and-return rather than a reach for the mouse.
          if (e.key === "Enter" && places.length) {
            e.preventDefault();
            pick(places[0]);
          }
        }}
        className="field w-full"
      />

      {status === "searching" && (
        <p className="mt-1 font-data text-[10px] text-ink-soft">Looking…</p>
      )}
      {status === "empty" && (
        <p className="mt-1 font-data text-[10px] text-ink-soft">
          Nothing found. Try the village rather than the viewpoint, or drop it on the map.
        </p>
      )}
      {status === "error" && (
        <p className="mt-1 font-data text-[10px] text-alert">{message}</p>
      )}

      {places.length > 0 && (
        <ul className="mt-1 border border-paper-edge">
          {places.map((p) => (
            <li key={`${p.lat},${p.lon},${p.displayName}`}>
              <button
                type="button"
                onClick={() => pick(p)}
                className="block w-full border-b border-paper-edge px-2 py-1.5 text-left last:border-b-0 hover:bg-paper-edge/40"
              >
                <span className="text-[13px] font-bold">{p.name}</span>
                {p.kind && (
                  <span className="ml-1.5 font-data text-[9px] tracking-wider text-ink-soft uppercase">
                    {p.kind}
                  </span>
                )}
                {/* The full address is what separates two places of the same
                    name, which is the whole reason a list is shown at all. */}
                <span className="mt-0.5 block font-data text-[10px] leading-snug text-ink-soft">
                  {p.displayName}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
