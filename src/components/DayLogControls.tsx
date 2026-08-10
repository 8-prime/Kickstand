import type { Day, WxIndex } from "../types";
import { WX, WX_NAME } from "../data";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { logValue, useTripStore } from "../store/useTripStore";

/** What you actually rode, the weather, and a note.
 *
 *  Rendered in both the Route panel and the Roadbook list — they write to the
 *  same trip, so an entry made in one shows up in the other, and on everyone
 *  else's screen the next time they load. */
export function DayLogControls({ day }: { day: Day }) {
  const payload = useTripStore((s) => s.payload);
  const canEdit = payload?.access === "edit";

  const km = logValue(payload, day.n, "km");
  const wx = logValue(payload, day.n, "wx");
  const note = logValue(payload, day.n, "note");

  if (!canEdit) return <ReadOnlyLog km={km} wx={wx} note={note} />;
  return <EditableLog day={day} km={km} wx={wx} note={note} />;
}

function ReadOnlyLog({
  km,
  wx,
  note,
}: {
  km: number | string | null;
  wx: number | string | null;
  note: number | string | null;
}) {
  if (km === null && wx === null && note === null) {
    return <p className="font-data text-[10.5px] text-ink-soft">Nothing logged yet.</p>;
  }
  return (
    <p className="text-[13.5px] text-ink-soft">
      {km !== null && <span className="font-data font-bold text-ink">{km} km </span>}
      {wx !== null && <span>{WX[Number(wx) as WxIndex]} </span>}
      {note !== null && <span>{note}</span>}
    </p>
  );
}

function EditableLog({
  day,
  km,
  wx,
  note,
}: {
  day: Day;
  km: number | string | null;
  wx: number | string | null;
  note: number | string | null;
}) {
  const setLog = useTripStore((s) => s.setLog);
  const setWx = useTripStore((s) => s.setWx);

  // Typing is local; the write lands once you pause. Otherwise a note is one
  // HTTP request per keystroke.
  const kmField = useDebouncedValue<string>(km === null ? "" : String(km), (next) =>
    void setLog(day.n, "km", next === "" ? null : Number(next)),
  );
  const noteField = useDebouncedValue<string>((note as string) ?? "", (next) =>
    void setLog(day.n, "note", next === "" ? null : next),
  );

  return (
    <div className="flex flex-wrap items-start gap-x-4 gap-y-3">
      <label className="flex items-center gap-2">
        <span className="font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
          Actually rode
        </span>
        <input
          type="number"
          min={0}
          max={2000}
          inputMode="numeric"
          placeholder="km"
          className="field w-24 text-right"
          value={kmField.draft}
          onChange={(e) => kmField.change(e.target.value)}
          onBlur={kmField.flush}
        />
      </label>

      <div className="flex gap-px" role="group" aria-label="Weather">
        {WX.map((glyph, i) => {
          const on = wx === i;
          return (
            <button
              key={glyph}
              type="button"
              aria-label={WX_NAME[i]}
              aria-pressed={on}
              onClick={() => void setWx(day.n, i as WxIndex)}
              className={[
                "h-[31px] w-9 border text-sm transition-colors",
                on
                  ? "border-ink bg-ink text-paper"
                  : "border-paper-edge text-ink-soft hover:border-ink hover:text-ink",
              ].join(" ")}
            >
              {glyph}
            </button>
          );
        })}
      </div>

      <textarea
        rows={1}
        placeholder="Roads, weather, where you camped…"
        className="field min-w-[200px] flex-1 resize-y font-body"
        value={noteField.draft}
        onChange={(e) => noteField.change(e.target.value)}
        onBlur={noteField.flush}
      />
    </div>
  );
}
