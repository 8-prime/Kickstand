import type { TripDoc } from "../types";
import { DayLogControls } from "../components/DayLogControls";
import { Figure } from "../components/Stat";
import { km as fmtKm, hrs as fmtHrs, pad2, stamp } from "../lib/format";
import { loggedDays, loggedKm, useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";

/** The rally roadbook: every day in order, with somewhere to write down what
 *  actually happened. One shared log per trip — whoever gets to it first. */
export function RoadbookView({ doc }: { doc: TripDoc }) {
  const payload = useTripStore((s) => s.payload);
  const clearLog = useTripStore((s) => s.clearLog);
  const canEdit = payload?.access === "edit";
  const selectDay = useUiStore((s) => s.selectDay);
  const setView = useUiStore((s) => s.setView);

  const plannedKm = doc.days.reduce((s, d) => s + d.km, 0);
  const plannedHrs = doc.days.reduce((s, d) => s + d.hours, 0);
  const rideDays = doc.days.filter((d) => d.km > 0).length;
  const actual = loggedKm(payload);
  const done = loggedDays(payload);

  return (
    <div className="space-y-4">
      <section className="panel p-4">
        <div className="flex flex-wrap items-end justify-between gap-x-8 gap-y-2">
          <div>
            <p className="eyebrow">Odometer</p>
            <p className="font-data text-4xl leading-none font-bold tracking-tighter">
              {fmtKm(actual)}
              <span className="ml-2 font-data text-sm font-normal text-ink-soft">
                / {fmtKm(plannedKm)} km planned
              </span>
            </p>
          </div>
          <p className="font-data text-[11px] text-ink-soft">
            {done} of {rideDays} riding days logged · ~{fmtHrs(plannedHrs)} h moving planned
          </p>
        </div>
        <div className="mt-3 h-1.5 w-full bg-paper-deep">
          <div
            className="h-full bg-ride transition-[width] duration-500"
            style={{ width: `${plannedKm ? Math.min(100, (actual / plannedKm) * 100) : 0}%` }}
          />
        </div>
      </section>

      <ol className="space-y-3">
        {doc.days.map((d) => {
          const van = d.type === "van";
          const accent = van ? "var(--color-transfer)" : "var(--color-ride)";
          return (
            <li key={d.n} className="panel border-l-[3px] p-4" style={{ borderLeftColor: accent }}>
              <div className="flex flex-wrap items-center gap-3">
                <span className="font-data text-sm font-bold" style={{ color: accent }}>
                  {pad2(d.n)}
                </span>
                <span className="font-data text-[11px] tracking-[0.12em] text-ink-soft">
                  {stamp(d.date ?? "")}
                </span>
                <span
                  className="border px-2 py-0.5 font-data text-[9px] font-bold tracking-[0.14em] uppercase"
                  style={{ borderColor: accent, color: accent }}
                >
                  {van ? "Transfer" : "Ride"}
                </span>
                {d.stops.length > 1 && (
                  <button
                    type="button"
                    onClick={() => {
                      selectDay(d.n);
                      setView("route");
                    }}
                    className="ml-auto font-data text-[10px] tracking-[0.12em] text-ink-soft uppercase underline underline-offset-2 hover:text-ink"
                  >
                    Show on map
                  </button>
                )}
              </div>

              <h2 className="mt-1.5 text-xl">{d.title}</h2>
              {d.base && (
                <p className="mt-1 font-data text-[10px] tracking-[0.1em] text-transfer uppercase">
                  {d.base}
                </p>
              )}
              {d.detail && <p className="mt-2 text-[14.5px] text-ink-soft">{d.detail}</p>}

              <div className="mt-3 flex flex-wrap gap-x-7 gap-y-3">
                {d.km > 0 && <Figure k="Ride planned" v={`${fmtKm(d.km)} km`} />}
                {d.hours > 0 && <Figure k="Saddle time" v={`~${fmtHrs(d.hours)} h`} />}
                {d.van > 0 && <Figure k="Van transfer" v={`${fmtKm(d.van)} km`} />}
              </div>

              <div className="mt-3.5 border-t border-paper-edge pt-3.5">
                <DayLogControls day={d} />
              </div>
            </li>
          );
        })}
      </ol>

      {canEdit && (
        <ResetButton
          label="Clear the log for this trip"
          confirm={`Delete every logged distance, weather mark and note for ${doc.name}? This affects everyone with the link. The checklist is not touched.`}
          onConfirm={() => void clearLog()}
        />
      )}
    </div>
  );
}

export function ResetButton({
  label,
  confirm,
  onConfirm,
}: {
  label: string;
  confirm: string;
  onConfirm: () => void;
}) {
  return (
    <button
      type="button"
      onClick={() => {
        if (window.confirm(confirm)) onConfirm();
      }}
      className="border border-paper-edge px-3 py-1.5 font-data text-[10px] tracking-[0.12em] text-ink-soft uppercase transition-colors hover:border-alert hover:text-alert"
    >
      {label}
    </button>
  );
}
