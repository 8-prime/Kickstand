import { useState } from "react";
import type { TripDoc } from "../types";
import { useStopEditor } from "../hooks/useStopEditor";
import { useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";
import { Editable } from "./Editable";
import { PlaceSearch } from "./PlaceSearch";

/** The stops of one day, in the order they are ridden.
 *
 *  Order is not decoration: it is what the router is asked to go through, so
 *  reordering here is how a day gets reshaped. */
export function StopList({ doc, index }: { doc: TripDoc; index: number }) {
  const canEdit = useTripStore((s) => s.payload?.access === "edit");
  const editing = useUiStore((s) => s.editing);
  const setEditing = useUiStore((s) => s.setEditing);
  const { stops, add, remove, moveAt } = useStopEditor(doc, index);

  const [dragging, setDragging] = useState<number | null>(null);
  const [over, setOver] = useState<number | null>(null);

  if (!canEdit) return <ReadOnly names={stops.map((s) => s.name)} />;

  const drop = (to: number) => {
    if (dragging != null) void moveAt(dragging, to);
    setDragging(null);
    setOver(null);
  };

  return (
    <div className="mt-3.5 border-t border-dotted border-paper-edge pt-2.5">
      <div className="flex items-baseline justify-between gap-2">
        <p className="eyebrow">Stops</p>
        <button
          type="button"
          onClick={() => setEditing(!editing)}
          aria-pressed={editing}
          className="font-data text-[10px] tracking-[0.1em] text-ink-soft uppercase underline underline-offset-2 hover:text-ink"
        >
          {editing ? "Done on map" : "Place on map"}
        </button>
      </div>

      {stops.length === 0 && (
        <p className="mt-1 text-[13.5px] text-ink-soft italic">
          No stops yet. Search below, or turn on “Place on map” and click where the day goes.
        </p>
      )}

      <ol className="mt-1">
        {stops.map((stop, i) => (
          <li
            key={`${i}-${stop.lat},${stop.lon}`}
            draggable
            onDragStart={() => setDragging(i)}
            onDragEnd={() => {
              setDragging(null);
              setOver(null);
            }}
            onDragOver={(e) => {
              e.preventDefault();
              setOver(i);
            }}
            onDrop={(e) => {
              e.preventDefault();
              drop(i);
            }}
            className={[
              "flex items-baseline gap-2 border-b border-dotted border-paper-edge py-1 last:border-b-0",
              over === i && dragging !== i ? "bg-paper-edge/50" : "",
              dragging === i ? "opacity-40" : "",
            ].join(" ")}
          >
            <span
              aria-hidden
              title="Drag to reorder"
              className="cursor-grab font-data text-[11px] text-ink-soft select-none"
            >
              ⠿
            </span>
            <span className="w-4 shrink-0 font-data text-[10px] text-ink-soft">{i + 1}</span>

            <span className="min-w-0 flex-1 text-[13.5px]">
              <Editable
                path={`days[${index}].stops[${i}].name`}
                value={stop.name}
                placeholder="Name this stop"
              />
              <span className="ml-1.5 font-data text-[9.5px] text-ink-soft">
                {stop.lat.toFixed(3)}, {stop.lon.toFixed(3)}
              </span>
            </span>

            {/* Arrows as well as dragging: drag-and-drop cannot be done from a
                keyboard, and this is the only way to reorder a day. */}
            <span className="flex shrink-0 gap-0.5">
              <IconButton
                label={`Move ${stop.name || "this stop"} earlier`}
                disabled={i === 0}
                onClick={() => void moveAt(i, i - 1)}
              >
                ↑
              </IconButton>
              <IconButton
                label={`Move ${stop.name || "this stop"} later`}
                disabled={i === stops.length - 1}
                onClick={() => void moveAt(i, i + 1)}
              >
                ↓
              </IconButton>
              <IconButton
                label={`Remove ${stop.name || "this stop"}`}
                onClick={() => void remove(i)}
              >
                ✕
              </IconButton>
            </span>
          </li>
        ))}
      </ol>

      <PlaceSearch
        onPick={(p) => void add({ name: p.name, lat: p.lat, lon: p.lon })}
        placeholder="Search a place to add…"
      />
    </div>
  );
}

/** What a view link sees: the route as a sentence, with nothing to press. */
function ReadOnly({ names }: { names: string[] }) {
  const run = names.filter((x, i, a) => i === 0 || x !== a[i - 1]);
  if (run.length < 2) return null;
  return (
    <div className="mt-3.5 border-t border-dotted border-paper-edge pt-2.5">
      <p className="eyebrow">Stops</p>
      <p className="mt-1 text-[13.5px] leading-7 text-ink-soft">{run.join("  →  ")}</p>
    </div>
  );
}

function IconButton({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
      className="border border-paper-edge px-1.5 font-data text-[11px] leading-5 text-ink-soft hover:border-ink hover:text-ink disabled:opacity-30 disabled:hover:border-paper-edge disabled:hover:text-ink-soft"
    >
      {children}
    </button>
  );
}
