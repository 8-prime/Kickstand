import { useEffect } from "react";
import { Link, useParams } from "react-router-dom";
import { ConnectionStatus } from "../components/ConnectionStatus";
import { Spine } from "../components/Spine";
import { Stat } from "../components/Stat";
import { ViewTabs } from "../components/ViewTabs";
import { km as fmtKm, hrs as fmtHrs } from "../lib/format";
import { useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";
import { CampsView } from "../views/CampsView";
import { KitView } from "../views/KitView";
import { RoadbookView } from "../views/RoadbookView";
import { RouteView } from "../views/RouteView";
import { TripDataView } from "../views/TripDataView";

export function TripPage() {
  const { token = "" } = useParams();
  const load = useTripStore((s) => s.load);
  const status = useTripStore((s) => s.status);
  const error = useTripStore((s) => s.error);
  const payload = useTripStore((s) => s.payload);
  const view = useUiStore((s) => s.view);
  const resetUi = useUiStore((s) => s.reset);

  useEffect(() => {
    resetUi();
    void load(token);
  }, [token, load, resetUi]);

  if (status === "loading" && !payload) {
    return <Centered>Loading the trip…</Centered>;
  }

  if (status === "error" || !payload) {
    return (
      <Centered>
        <p className="text-[15px]">{error ?? "That trip could not be loaded."}</p>
        <Link
          to="/"
          className="mt-4 inline-block border border-ink px-3 py-1.5 font-data text-[11px] font-bold tracking-[0.12em] uppercase hover:bg-ink hover:text-paper"
        >
          All trips
        </Link>
      </Centered>
    );
  }

  const doc = payload.doc;
  const ridingKm = doc.days.reduce((s, d) => s + d.km, 0);
  const saddleHrs = doc.days.reduce((s, d) => s + d.hours, 0);

  return (
    <div className="mx-auto max-w-6xl px-4 pt-6 pb-16 sm:px-6">
      <ConnectionStatus />

      <header className="flex flex-wrap items-end justify-between gap-x-8 gap-y-4">
        <div>
          <p className="eyebrow">
            {doc.origin ? `${doc.origin} → ` : ""}
            {doc.subtitle || doc.slug}
            {doc.dates ? ` · ${doc.dates}` : ""}
          </p>
          <h1 className="mt-1 text-[clamp(1.75rem,5vw,2.75rem)]">{doc.name}</h1>
        </div>
        <div className="flex items-center gap-4">
          {payload.access === "view" && (
            <span className="border border-paper-edge px-2 py-1 font-data text-[9px] tracking-[0.14em] text-ink-soft uppercase">
              Read only
            </span>
          )}
          <Link
            to="/"
            className="font-data text-[10px] tracking-[0.12em] text-ink-soft uppercase underline underline-offset-2 hover:text-ink"
          >
            All trips
          </Link>
        </div>
      </header>

      <div className="mt-6 flex flex-wrap gap-x-10 gap-y-4">
        <Stat k="Riding" v={fmtKm(ridingKm)} u="km" tone="ride" />
        <Stat k="Saddle time" v={fmtHrs(saddleHrs)} u="h" tone="ride" />
        <Stat k="Van down" v={fmtKm(doc.vanIn)} u="km" tone="transfer" />
        <Stat k="Van home" v={fmtKm(doc.vanOut)} u="km" tone="transfer" />
      </div>

      <div className="rule mt-4 mb-5" />

      <Spine doc={doc} />

      <div className="mt-7 mb-4">
        <ViewTabs />
      </div>

      <main>
        {view === "route" && <RouteView doc={doc} payload={payload} />}
        {view === "roadbook" && <RoadbookView doc={doc} />}
        {view === "kit" && <KitView doc={doc} />}
        {view === "camps" && <CampsView doc={doc} />}
        {view === "trip" && <TripDataView doc={doc} payload={payload} />}
      </main>

      <footer className="mt-10 border-t border-paper-edge pt-4 font-data text-[10.5px] leading-relaxed text-ink-soft">
        <p>
          Every stop is a real coordinate. Route lines follow real roads once the server has
          fetched them; until then they are schematic and drawn broken. Distances and saddle
          times are estimates made at planning time, not measurements.
        </p>
        <p className="mt-1.5">
          The log and checklist are shared by everyone with this link. A copy of the trip is
          kept in this browser so it still opens with no signal.
        </p>
      </footer>
    </div>
  );
}

function Centered({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center px-4 text-center">
      {children}
    </div>
  );
}
