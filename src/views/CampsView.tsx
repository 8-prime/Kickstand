import type { TripDoc } from "../types";
import { Editable } from "../components/Editable";
import { useTripStore } from "../store/useTripStore";
import { useUiStore } from "../store/useUiStore";

/** Where you sleep. A base with nothing chosen is shown as an open task, not
 *  left out — a gap you can act on beats a gap you have to notice. */
export function CampsView({ doc }: { doc: TripDoc }) {
  const camps = doc.campsites ?? [];
  const rejected = doc.rejectedCampsites ?? [];
  const canEdit = useTripStore((s) => s.payload?.access === "edit");
  const patch = useTripStore((s) => s.patch);
  const setView = useUiStore((s) => s.setView);
  const outstanding = camps.filter((c) => c.status === "not-researched").length;

  return (
    <div className="space-y-4">
      {doc.campsiteCaveat && (
        <section className="panel border-l-[3px] border-l-alert p-4">
          <p className="eyebrow text-alert">Before you rely on any of this</p>
          <div className="mt-1.5 text-[14.5px] text-ink-soft">
            <Editable path="campsiteCaveat" value={doc.campsiteCaveat} kind="multiline" />
          </div>
        </section>
      )}

      <div className="grid gap-3 md:grid-cols-2">
        {camps.map((c, i) => (
          <article key={`${c.base}-${i}`} className="panel flex flex-col p-4">
            <div className="flex items-baseline justify-between gap-3">
              <span className="font-data text-[10px] tracking-[0.16em] text-transfer uppercase">
                Base {c.base} · {c.baseName}
              </span>
              {c.status === "picked" && (
                <span className="font-data text-[11px] font-bold">
                  <Editable
                    path={`campsites[${i}].rating`}
                    value={c.rating}
                    kind="number"
                    className="w-14"
                    emptyLabel="—"
                  />
                  ★ <span className="font-normal text-ink-soft">({c.reviews ?? 0})</span>
                </span>
              )}
            </div>

            {c.status === "picked" ? (
              <>
                <h2 className="mt-1.5 text-lg">
                  <Editable path={`campsites[${i}].name`} value={c.name} />
                </h2>
                <p className="mt-1 font-data text-[13px]">
                  {canEdit ? (
                    <Editable
                      path={`campsites[${i}].phone`}
                      value={c.phone}
                      placeholder="Phone number"
                    />
                  ) : (
                    c.phone && (
                      <a
                        href={`tel:${c.phone.replace(/\s/g, "")}`}
                        className="text-ink underline underline-offset-2 hover:text-alert"
                      >
                        {c.phone}
                      </a>
                    )
                  )}
                </p>
                <div className="mt-2 text-[13.5px] text-ink-soft">
                  <Editable
                    path={`campsites[${i}].note`}
                    value={c.note}
                    kind="multiline"
                    placeholder="What is worth knowing about this site?"
                  />
                </div>

                {c.closingDateVerified ? (
                  <p className="mt-2.5 font-data text-[10px] tracking-[0.1em] text-transfer uppercase">
                    Closing date confirmed
                  </p>
                ) : (
                  <div className="mt-2.5 flex items-center gap-3">
                    <p className="font-data text-[10px] tracking-[0.1em] text-alert uppercase">
                      Closing date unconfirmed — call them
                    </p>
                    {canEdit && (
                      <button
                        type="button"
                        onClick={() =>
                          void patch(`campsites[${i}].closingDateVerified`, true)
                        }
                        className="font-data text-[10px] tracking-[0.1em] text-ink-soft uppercase underline underline-offset-2 hover:text-ink"
                      >
                        I called, it is open
                      </button>
                    )}
                  </div>
                )}
              </>
            ) : (
              <>
                <h2 className="mt-1.5 text-lg text-ink-soft">No campsite picked yet</h2>
                <p className="mt-2 text-[13.5px] text-ink-soft">
                  Nothing has been researched for this base. You need somewhere that takes the
                  bikes plus a transporter, and that is still open on your dates.
                </p>
                <p className="mt-2.5 font-data text-[10px] tracking-[0.1em] text-alert uppercase">
                  Open task
                </p>
                {canEdit && (
                  <button
                    type="button"
                    onClick={() => {
                      const name = window.prompt("Campsite name?");
                      if (!name) return;
                      void patch(`campsites[${i}].name`, name).then(() =>
                        patch(`campsites[${i}].status`, "picked"),
                      );
                    }}
                    className="mt-3 self-start border border-ink px-3 py-1.5 font-data text-[10px] font-bold tracking-[0.12em] uppercase transition-colors hover:bg-ink hover:text-paper"
                  >
                    Found one
                  </button>
                )}
              </>
            )}
          </article>
        ))}
      </div>

      {rejected.length > 0 && (
        <section className="panel p-4">
          <p className="eyebrow">Looked at and turned down</p>
          <ul className="mt-2 space-y-1 text-[13.5px] text-ink-soft">
            {rejected.map((r, i) => (
              <li key={i}>
                <Editable path={`rejectedCampsites[${i}]`} value={r} />
              </li>
            ))}
          </ul>
          <p className="mt-2.5 font-data text-[10.5px] text-ink-soft">
            Kept so nobody researches the same ground twice.
          </p>
        </section>
      )}

      {outstanding > 0 && (
        <p className="font-data text-[10.5px] leading-relaxed text-ink-soft">
          {outstanding} of {camps.length} bases still need a campsite. Until they have one, the{" "}
          <button
            type="button"
            onClick={() => setView("route")}
            className="underline underline-offset-2 hover:text-ink"
          >
            map
          </button>{" "}
          shows the base town rather than a pitch.
        </p>
      )}
    </div>
  );
}
