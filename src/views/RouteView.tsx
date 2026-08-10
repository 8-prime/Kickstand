import type { TripDoc, TripPayload } from "../types";
import { DayPanel } from "../components/DayPanel";
import { RouteMap } from "../components/RouteMap";

export function RouteView({ doc, payload }: { doc: TripDoc; payload: TripPayload }) {
  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1.55fr)_minmax(320px,1fr)]">
      <RouteMap doc={doc} payload={payload} />
      <DayPanel doc={doc} payload={payload} />
    </div>
  );
}
