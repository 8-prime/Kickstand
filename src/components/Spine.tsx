import { useMemo } from "react";
import type { TripDoc, WxIndex } from "../types";
import { WX, WX_NAME } from "../data";
import { km as fmtKm, pad2 } from "../lib/format";
import { logValue, useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";

/** The spine.
 *
 *  One segment per day, width proportional to the distance covered that day —
 *  ridden or driven. It doubles as the app's navigation and as the progress
 *  meter: once you log actual km, each riding segment fills to match.
 *
 *  Reading it left to right shows the argument behind the whole plan. The two
 *  van bookends dwarf everything else, which is why the long drive is run
 *  first and the route works back toward home. */
export function Spine({ doc }: { doc: TripDoc }) {
  const selectedDay = useUiStore((s) => s.selectedDay);
  const toggleDay = useUiStore((s) => s.toggleDay);
  const payload = useTripStore((s) => s.payload);

  const weights = useMemo(
    () => doc.days.map((d) => Math.max(d.km + d.van, 60)),
    [doc.days],
  );

  const baseByDay = useMemo(
    () => new Map(doc.bases.map((b) => [b.arriveDay, b])),
    [doc.bases],
  );

  return (
    <section aria-label="Trip spine" className="select-none">
      <div className="mb-1.5 flex items-baseline justify-between gap-4">
        <span className="eyebrow">The spine · segment width is distance</span>
        <span className="eyebrow hidden sm:inline">
          {doc.days.length} days · {doc.bases.length} base camps
        </span>
      </div>

      <div className="flex items-end gap-px">
        {doc.days.map((d, i) => {
          const on = selectedDay === d.n;
          const wx = logValue(payload, d.n, "wx");
          const logged = Number(logValue(payload, d.n, "km")) || 0;
          const fill = d.km > 0 ? Math.min(100, (logged / d.km) * 100) : 0;
          const total = d.km + d.van;

          return (
            <button
              key={d.n}
              type="button"
              onClick={() => toggleDay(d.n)}
              aria-pressed={on}
              // The visible label is just the day number, which tells a screen
              // reader nothing. Spell it out.
              aria-label={`Day ${d.n}, ${d.title}, ${fmtKm(total)} km${
                wx !== null ? `, logged ${WX_NAME[Number(wx) as WxIndex]}` : ""
              }`}
              title={`Day ${d.n} · ${d.title} · ${fmtKm(total)} km`}
              // items-stretch is not the default here: a <button> carries a UA
              // align-items that would collapse the bar to zero width.
              className="group relative flex flex-col items-stretch justify-end"
              style={{ flexGrow: weights[i], flexBasis: 0, minWidth: 22 }}
            >
              <span
                className="mb-1 h-3 text-center font-data text-[9px] leading-3 text-alert"
                aria-hidden
              >
                {wx !== null ? WX[Number(wx) as WxIndex] : ""}
              </span>

              <span
                className={[
                  "relative block overflow-hidden border transition-[height,opacity,border-color]",
                  on
                    ? "border-ink opacity-100"
                    : "border-paper-edge opacity-70 group-hover:opacity-100",
                ].join(" ")}
                style={{
                  height: on ? 40 : 32,
                  background:
                    d.type === "van"
                      ? // Hatched: the van moving, nobody riding.
                        `repeating-linear-gradient(-45deg, var(--color-transfer) 0 2px, transparent 2px 6px)`
                      : `color-mix(in srgb, var(--color-ride) 22%, transparent)`,
                }}
              >
                {fill > 0 && (
                  <span
                    className="absolute inset-y-0 left-0 bg-ride transition-[width] duration-500"
                    style={{ width: `${fill}%` }}
                  />
                )}
              </span>

              <span
                className={[
                  "mt-1 block text-center font-data text-[10px] leading-none",
                  on ? "font-bold text-ink" : "text-ink-soft",
                ].join(" ")}
              >
                {pad2(d.n)}
              </span>
            </button>
          );
        })}
      </div>

      {/* Base-camp flags, aligned to the same weights. */}
      <div className="mt-1 flex gap-px" aria-hidden>
        {doc.days.map((d, i) => {
          const base = baseByDay.get(d.n);
          return (
            <div
              key={d.n}
              className="relative h-4"
              style={{ flexGrow: weights[i], flexBasis: 0, minWidth: 22 }}
            >
              {base && (
                <span className="absolute top-0 left-0 flex items-center gap-1 whitespace-nowrap font-data text-[9px] leading-4 text-transfer">
                  <span className="text-ink">▲</span>
                  {base.index}
                  {/* Names collide once the segments get narrow. */}
                  <span className="hidden sm:inline">{base.name}</span>
                </span>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
