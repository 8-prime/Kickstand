import { useUiStore, type ViewId } from "../store/useUiStore";

const TABS: { id: ViewId; label: string; hint: string }[] = [
  { id: "route", label: "Route", hint: "Map and the day you have selected" },
  { id: "roadbook", label: "Roadbook", hint: "Log what you actually rode" },
  { id: "kit", label: "Kit", hint: "Pre-departure checklist" },
  { id: "camps", label: "Camps", hint: "Campsites, picked and still open" },
  { id: "trip", label: "Trip", hint: "Import, export and share this trip" },
];

export function ViewTabs() {
  const view = useUiStore((s) => s.view);
  const setView = useUiStore((s) => s.setView);

  return (
    <nav aria-label="Views" className="flex flex-wrap gap-px">
      {TABS.map((t) => {
        const on = t.id === view;
        return (
          <button
            key={t.id}
            type="button"
            aria-current={on ? "page" : undefined}
            title={t.hint}
            onClick={() => setView(t.id)}
            className={[
              "border px-4 py-1.5 font-data text-[11px] font-bold tracking-[0.14em] uppercase transition-colors",
              on
                ? "border-ink bg-ink text-paper"
                : "border-paper-edge text-ink-soft hover:border-ink hover:text-ink",
            ].join(" ")}
          >
            {t.label}
          </button>
        );
      })}
    </nav>
  );
}
