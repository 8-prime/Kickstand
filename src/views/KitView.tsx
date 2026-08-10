import type { TripDoc } from "../types";
import { Editable } from "../components/Editable";
import { kitChecked, useTripStore } from "../store/useTripStore";
import { ResetButton } from "./RoadbookView";

/** Pre-departure checklist. The legal group is first and marked, because those
 *  items carry a fine rather than an inconvenience. */
export function KitView({ doc }: { doc: TripDoc }) {
  const groups = doc.kit ?? [];
  const payload = useTripStore((s) => s.payload);
  const toggleKit = useTripStore((s) => s.toggleKit);
  const clearKit = useTripStore((s) => s.clearKit);
  const canEdit = payload?.access === "edit";

  const all = groups.flatMap((g) => g.items);
  const ticked = all.filter((i) => kitChecked(payload, i.id)).length;

  if (!groups.length) {
    return (
      <section className="panel p-4">
        <h2 className="text-lg">No checklist yet</h2>
        <p className="mt-2 text-[14.5px] text-ink-soft">
          The checklist lives in the trip document, under <code>kit</code>. Add one from the
          Trip tab — export the JSON, add the groups, and import it back.
        </p>
      </section>
    );
  }

  return (
    <div className="space-y-4">
      <section className="panel flex flex-wrap items-end justify-between gap-x-8 gap-y-2 p-4">
        <div>
          <p className="eyebrow">Packed</p>
          <p className="font-data text-4xl leading-none font-bold tracking-tighter">
            {ticked}
            <span className="ml-1 font-data text-sm font-normal text-ink-soft">
              / {all.length}
            </span>
          </p>
        </div>
        <p className="max-w-md font-data text-[10.5px] leading-relaxed text-ink-soft">
          Shared with everyone on this trip. Ticking something here ticks it for the whole
          group.
        </p>
      </section>

      {groups.map((g, gi) => {
        const groupTicked = g.items.filter((i) => kitChecked(payload, i.id)).length;
        return (
          <section
            key={g.group}
            className={["panel p-4", g.legal ? "border-l-[3px] border-l-alert" : ""].join(" ")}
          >
            <div className="flex flex-wrap items-baseline justify-between gap-3">
              <h2 className="text-lg">
                <Editable path={`kit[${gi}].group`} value={g.group} />
              </h2>
              <span className="font-data text-[10px] tracking-[0.14em] text-ink-soft">
                {groupTicked}/{g.items.length}
              </span>
            </div>

            <ul className="mt-3 divide-y divide-paper-edge">
              {g.items.map((item, ii) => {
                const on = kitChecked(payload, item.id);
                return (
                  <li key={item.id} className="flex items-start gap-3 py-2.5">
                    <input
                      type="checkbox"
                      id={`kit-${item.id}`}
                      checked={on}
                      disabled={!canEdit}
                      onChange={() => void toggleKit(item.id)}
                      className="mt-1 size-4 shrink-0 accent-ride"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <label
                          htmlFor={`kit-${item.id}`}
                          className={[
                            "cursor-pointer font-medium",
                            on ? "text-ink-soft line-through" : "",
                          ].join(" ")}
                        >
                          {item.title}
                        </label>
                        {item.flag && (
                          <span className="border border-alert px-1.5 py-px font-data text-[9px] font-bold tracking-[0.1em] text-alert">
                            {item.flag}
                          </span>
                        )}
                      </div>
                      {(item.why || canEdit) && (
                        <div className="mt-1 text-[13px] leading-relaxed text-ink-soft">
                          <Editable
                            path={`kit[${gi}].items[${ii}].why`}
                            value={item.why}
                            kind="multiline"
                            placeholder="Why does this matter?"
                            emptyLabel=""
                          />
                        </div>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
          </section>
        );
      })}

      {canEdit && (
        <ResetButton
          label="Untick everything for this trip"
          confirm={`Untick every item on the ${doc.name} checklist? This affects everyone with the link. Your ride log is not touched.`}
          onConfirm={() => void clearKit()}
        />
      )}
    </div>
  );
}
