import type { TripDoc, TripPayload } from "../types";
import { useDayGeometry } from "../hooks/useDayGeometry";
import { km as fmtKm, pad2, stamp } from "../lib/format";
import { useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";
import { DayLogControls } from "./DayLogControls";
import { Editable } from "./Editable";
import { Figure } from "./Stat";

/** What routed distance actually measures. Said once, where the number is. */
const ROUTED_CAVEAT =
  "This is the shortest car route through the listed stops, not the scenic line the day is planned around — expect it to read low wherever the plan takes the long way. The drawn roads are real; the planned distance is the one to fuel and time against.";

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

  const names = day.stops
    .map((p) => p.name)
    .filter((x, i, a) => i === 0 || x !== a[i - 1]);

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
          {stamp(day.date ?? "")}
        </span>
        <span
          className="ml-auto border px-2 py-0.5 font-data text-[9px] font-bold tracking-[0.14em] uppercase"
          style={{
            borderColor: van ? "var(--color-transfer)" : "var(--color-ride)",
            color: van ? "var(--color-transfer)" : "var(--color-ride)",
          }}
        >
          {van ? "Transfer" : "Ride"}
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
          <Figure k="Direct via stops" v={`${fmtKm(geo.routedKm)} km`} />
        )}
      </div>

      {day.stops.length > 1 && (
        <div className="mt-3.5 border-t border-dotted border-paper-edge pt-2.5">
          <p className="eyebrow">Stops</p>
          <p className="mt-1 text-[13.5px] leading-7 text-ink-soft">{names.join("  →  ")}</p>
          <p className="mt-1 font-data text-[10px] text-ink-soft">
            Stops are edited in the trip JSON — see the Trip tab.
          </p>
        </div>
      )}

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

      <div className="mt-4 border-t border-paper-edge pt-3.5">
        <p className="eyebrow mb-2">Log this day</p>
        <DayLogControls day={day} />
      </div>
    </article>
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
  const canEdit = useTripStore((s) => s.payload?.access === "edit");
  const routed = useTripStore((s) => s.payload?.routes.length ?? 0);
  const routable = doc.days.filter((d) => d.stops.length >= 2).length;

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
            <button
              type="button"
              disabled={!!routing}
              onClick={() => void refreshRoutes()}
              className="mt-2 border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper disabled:opacity-50"
            >
              {routing ? "Routing…" : "Fetch missing routes"}
            </button>
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
