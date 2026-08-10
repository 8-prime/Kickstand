import type { DayType, TripDoc, TripPayload } from "../types";
import { useDayGeometry } from "../hooks/useDayGeometry";
import { hrs, km as fmtKm, pad2, stamp } from "../lib/format";
import { useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";
import { DayLogControls } from "./DayLogControls";
import { Editable } from "./Editable";
import { Figure } from "./Stat";
import { StopList } from "./StopList";

/** What routed distance actually measures. Said once, where the number is. */
const ROUTED_CAVEAT =
  "This is the shortest car route through the listed stops, not the scenic line the day is planned around — expect it to read low wherever the plan takes the long way. The drawn roads are real; the planned distance is the one to fuel and time against, which is why taking these figures is a button rather than something that happens to you.";

/** How far the routed figure has to be from the planned one before it is worth
 *  offering. Below this the difference is rounding, and a button that changes
 *  180 to 181 is noise. */
const SUGGEST_THRESHOLD = 0.02;

export function DayPanel({ doc, payload }: { doc: TripDoc; payload: TripPayload }) {
  const selectedDay = useUiStore((s) => s.selectedDay);
  const day = selectedDay ? doc.days.find((d) => d.n === selectedDay) : undefined;

  if (!day) return <Overview doc={doc} />;
  return <Detail doc={doc} payload={payload} dayN={day.n} />;
}

function Detail({
  doc,
  payload,
  dayN,
}: {
  doc: TripDoc;
  payload: TripPayload;
  dayN: number;
}) {
  const index = doc.days.findIndex((d) => d.n === dayN);
  const day = doc.days[index];
  const geo = useDayGeometry(payload, day);
  const refreshRoutes = useTripStore((s) => s.refreshRoutes);
  const routing = useTripStore((s) => s.routing);
  const canEdit = payload.access === "edit";
  const van = day.type === "van";

  return (
    <article
      className="panel border-l-[3px] p-4"
      style={{ borderLeftColor: van ? "var(--color-transfer)" : "var(--color-ride)" }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span
          className="font-data text-sm font-bold"
          style={{ color: van ? "var(--color-transfer)" : "var(--color-ride)" }}
        >
          {pad2(day.n)}
        </span>
        <span className="font-data text-[11px] tracking-[0.12em] text-ink-soft">
          {canEdit ? (
            <Editable
              path={`days[${index}].date`}
              value={day.date}
              placeholder="Which date?"
              className="w-28"
            />
          ) : (
            stamp(day.date ?? "")
          )}
        </span>
        <span className="ml-auto">
          <TypeToggle index={index} type={day.type} canEdit={canEdit} />
        </span>
      </div>

      <h2 className="mt-1.5 text-2xl">
        <Editable path={`days[${index}].title`} value={day.title} />
      </h2>
      <p className="mt-1 font-data text-[10px] tracking-[0.1em] text-transfer uppercase">
        <Editable
          path={`days[${index}].base`}
          value={day.base}
          placeholder="Which base do you sleep at?"
        />
      </p>
      <div className="mt-2.5 text-[14.5px] text-ink-soft">
        <Editable
          path={`days[${index}].detail`}
          value={day.detail}
          kind="multiline"
          placeholder="What is this day actually like?"
        />
      </div>

      <div className="mt-3.5 flex flex-wrap gap-x-7 gap-y-3">
        <EditableFigure
          k="Ride planned"
          path={`days[${index}].km`}
          value={day.km}
          unit="km"
          hide={van && day.km === 0}
        />
        <EditableFigure
          k="Saddle time"
          path={`days[${index}].hours`}
          value={day.hours}
          unit="h"
          hide={day.hours === 0 && !canEdit}
        />
        <EditableFigure
          k="Van"
          path={`days[${index}].van`}
          value={day.van}
          unit="km"
          hide={day.van === 0 && !canEdit}
        />
        {geo.routedKm != null && (
          <Figure
            k="Direct via stops"
            v={`${fmtKm(geo.routedKm)} km${geo.routedHours ? ` · ${hrs(geo.routedHours)} h` : ""}`}
          />
        )}
      </div>

      {canEdit && (
        <RoutedSuggestion
          index={index}
          plannedKm={van ? day.van : day.km}
          plannedHours={day.hours}
          routedKm={geo.routedKm}
          routedHours={geo.routedHours}
          field={van ? "van" : "km"}
        />
      )}

      <StopList doc={doc} index={index} />

      {day.stops.length > 1 && (
        <p className="mt-3 font-data text-[10.5px] leading-relaxed text-ink-soft">
          {geo.routed ? (
            ROUTED_CAVEAT
          ) : (
            <>
              Showing a schematic line — it links the stops but is not the road.{" "}
              {canEdit && (
                <button
                  type="button"
                  disabled={!!routing}
                  onClick={() => void refreshRoutes([day.n])}
                  className="underline underline-offset-2 hover:text-ink disabled:opacity-50"
                >
                  {routing ? "Routing…" : "Fetch the real route"}
                </button>
              )}
            </>
          )}
        </p>
      )}

      {canEdit && <DayOps doc={doc} dayN={day.n} title={day.title} />}

      <div className="mt-4 border-t border-paper-edge pt-3.5">
        <p className="eyebrow mb-2">Log this day</p>
        <DayLogControls day={day} />
      </div>
    </article>
  );
}

/** Offers the routed figures for the planned ones, once they differ enough to
 *  be worth a click.
 *
 *  A suggestion rather than an overwrite: planned, routed and logged are three
 *  different numbers on purpose, and the planned one is somebody's decision to
 *  take the long way round. */
function RoutedSuggestion({
  index,
  plannedKm,
  plannedHours,
  routedKm,
  routedHours,
  field,
}: {
  index: number;
  plannedKm: number;
  plannedHours: number;
  routedKm: number | null;
  routedHours: number | null;
  field: "km" | "van";
}) {
  const patchMany = useTripStore((s) => s.patchMany);
  if (routedKm == null) return null;

  const kmOff = plannedKm === 0 || Math.abs(routedKm - plannedKm) / plannedKm > SUGGEST_THRESHOLD;
  const hoursOff =
    routedHours != null &&
    (plannedHours === 0 || Math.abs(routedHours - plannedHours) / plannedHours > SUGGEST_THRESHOLD);
  if (!kmOff && !hoursOff) return null;

  const apply = () => {
    const ops: { path: string; value: unknown }[] = [
      { path: `days[${index}].${field}`, value: routedKm },
    ];
    if (routedHours != null) ops.push({ path: `days[${index}].hours`, value: routedHours });
    void patchMany(ops);
  };

  return (
    <p className="mt-2 font-data text-[10.5px] text-ink-soft">
      ↳{" "}
      <button
        type="button"
        onClick={apply}
        className="border border-paper-edge px-2 py-0.5 font-bold tracking-[0.08em] uppercase hover:border-ink hover:text-ink"
      >
        Use {fmtKm(routedKm)} km
        {routedHours != null && ` · ${hrs(routedHours)} h`}
      </button>{" "}
      as the plan
    </p>
  );
}

/** Ride or transfer. Two buttons rather than a field: it is a choice between
 *  two things, and it recolours the day's line the moment it changes. */
function TypeToggle({
  index,
  type,
  canEdit,
}: {
  index: number;
  type: DayType;
  canEdit: boolean;
}) {
  const patch = useTripStore((s) => s.patch);
  const van = type === "van";
  const colour = van ? "var(--color-transfer)" : "var(--color-ride)";

  if (!canEdit) {
    return (
      <span
        className="border px-2 py-0.5 font-data text-[9px] font-bold tracking-[0.14em] uppercase"
        style={{ borderColor: colour, color: colour }}
      >
        {van ? "Transfer" : "Ride"}
      </span>
    );
  }

  return (
    <span className="flex gap-px" role="group" aria-label="Day type">
      {(["ride", "van"] as DayType[]).map((t) => {
        const on = type === t;
        const c = t === "van" ? "var(--color-transfer)" : "var(--color-ride)";
        return (
          <button
            key={t}
            type="button"
            aria-pressed={on}
            onClick={() => !on && void patch(`days[${index}].type`, t)}
            className="border px-2 py-0.5 font-data text-[9px] font-bold tracking-[0.14em] uppercase"
            style={
              on
                ? { borderColor: c, background: c, color: "var(--color-paper)" }
                : { borderColor: "var(--color-paper-edge)", color: "var(--color-ink-soft)" }
            }
          >
            {t === "van" ? "Transfer" : "Ride"}
          </button>
        );
      })}
    </span>
  );
}

/** Add, remove and move whole days.
 *
 *  Online only, and said so rather than silently queued: every other edit names
 *  a field and merges, but this renumbers the days that naming depends on. */
function DayOps({ doc, dayN, title }: { doc: TripDoc; dayN: number; title: string }) {
  const dayOp = useTripStore((s) => s.dayOp);
  const online = useTripStore((s) => s.online);
  const selectDay = useUiStore((s) => s.selectDay);
  const last = doc.days.length;

  const remove = () => {
    const ok = window.confirm(
      `Delete day ${dayN}, “${title}”?\n\n` +
        `The days after it move up a number, and anything logged against this one — ` +
        `distance, weather, notes — goes with it.`,
    );
    if (!ok) return;
    void dayOp({ op: "delete", day: dayN }).then(() => selectDay(null));
  };

  return (
    <div className="mt-4 border-t border-paper-edge pt-3">
      <p className="eyebrow mb-1.5">This day in the trip</p>
      <div className="flex flex-wrap gap-1">
        <OpButton
          disabled={!online || dayN === 1}
          onClick={() => void dayOp({ op: "move", day: dayN, to: dayN - 1 }).then(() => selectDay(dayN - 1))}
        >
          ← Earlier
        </OpButton>
        <OpButton
          disabled={!online || dayN === last}
          onClick={() => void dayOp({ op: "move", day: dayN, to: dayN + 1 }).then(() => selectDay(dayN + 1))}
        >
          Later →
        </OpButton>
        <OpButton
          disabled={!online}
          onClick={() => void dayOp({ op: "insert", after: dayN }).then(() => selectDay(dayN + 1))}
        >
          + Day after
        </OpButton>
        <OpButton disabled={!online || last === 1} onClick={remove} alert>
          Delete day {dayN}
        </OpButton>
      </div>
      {!online && (
        <p className="mt-1.5 font-data text-[10px] text-ink-soft">
          These need a connection — they renumber the whole trip, so they cannot be queued.
        </p>
      )}
    </div>
  );
}

function OpButton({
  onClick,
  disabled,
  alert,
  children,
}: {
  onClick: () => void;
  disabled?: boolean;
  alert?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={[
        "border px-2 py-1 font-data text-[10px] font-bold tracking-[0.1em] uppercase transition-colors disabled:opacity-35",
        alert
          ? "border-paper-edge text-alert hover:border-alert enabled:hover:bg-alert enabled:hover:text-paper"
          : "border-paper-edge text-ink-soft hover:border-ink hover:text-ink",
      ].join(" ")}
    >
      {children}
    </button>
  );
}

function EditableFigure({
  k,
  path,
  value,
  unit,
  hide,
}: {
  k: string;
  path: string;
  value: number;
  unit: string;
  hide?: boolean;
}) {
  if (hide) return null;
  return (
    <div>
      <span className="block font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
        {k}
      </span>
      <span className="font-data text-[15px] font-bold">
        <Editable path={path} value={value} kind="number" className="w-20" /> {unit}
      </span>
    </div>
  );
}

function Overview({ doc }: { doc: TripDoc }) {
  const ridingKm = doc.days.reduce((s, d) => s + d.km, 0);
  const routing = useTripStore((s) => s.routing);
  const refreshRoutes = useTripStore((s) => s.refreshRoutes);
  const dayOp = useTripStore((s) => s.dayOp);
  const online = useTripStore((s) => s.online);
  const canEdit = useTripStore((s) => s.payload?.access === "edit");
  const routed = useTripStore((s) => s.payload?.routes.length ?? 0);
  const routable = doc.days.filter((d) => d.stops.length >= 2).length;
  const selectDay = useUiStore((s) => s.selectDay);

  return (
    <article className="panel border-l-[3px] border-l-ink p-4">
      <p className="eyebrow">
        {doc.days.length} days · {doc.bases.length} base camps · {fmtKm(ridingKm)} km riding
      </p>
      <h2 className="mt-1.5 text-2xl">Pick a day, or click a route on the map</h2>
      <p className="mt-2 text-[14.5px] text-ink-soft">
        Numbered pins are the base camps — {doc.bases.map((b) => b.name).join(", ")}. The
        dashed green line is the transporter moving between them; rose loops are what you ride
        from each camp and come back to at night.
      </p>
      {doc.campsiteCaveat && (
        <p className="mt-2 text-[14.5px] text-ink-soft">{doc.campsiteCaveat}</p>
      )}

      <div className="mt-4 border-t border-paper-edge pt-3.5">
        <p className="font-data text-[10.5px] text-ink-soft">
          {routed} of {routable} riding days have real road geometry.
        </p>
        {canEdit && (
          <>
            <div className="mt-2 flex flex-wrap gap-1.5">
              <button
                type="button"
                disabled={!!routing}
                onClick={() => void refreshRoutes()}
                className="border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper disabled:opacity-50"
              >
                {routing ? "Routing…" : "Fetch missing routes"}
              </button>
              <OpButton
                disabled={!online}
                onClick={() =>
                  void dayOp({ op: "insert", after: doc.days.length }).then(() =>
                    selectDay(doc.days.length + 1),
                  )
                }
              >
                + Day at the end
              </OpButton>
            </div>
            <p className="mt-2 font-data text-[10.5px] leading-relaxed text-ink-soft">
              The server fetches them one at a time and keeps them, so everyone on the trip
              gets the result and the map still draws with no signal.
            </p>
          </>
        )}
      </div>
    </article>
  );
}
