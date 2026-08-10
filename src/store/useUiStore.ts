import { create } from "zustand";

export type ViewId = "route" | "roadbook" | "kit" | "camps" | "trip";
export type BasemapId = "topo" | "light" | "osm";

interface UiState {
  /** Day number, or null for the whole trip. */
  selectedDay: number | null;
  view: ViewId;
  basemap: BasemapId;
  showCamps: boolean;
  /** True while the map's markers are draggable and clicks place stops.
   *  A mode rather than an always-on affordance: the same click that adds a
   *  waypoint is the one you use to pan and read the map the rest of the time. */
  editing: boolean;
  selectDay: (n: number | null) => void;
  toggleDay: (n: number) => void;
  setView: (v: ViewId) => void;
  setBasemap: (b: BasemapId) => void;
  toggleCamps: () => void;
  setEditing: (on: boolean) => void;
  toggleEditing: () => void;
  reset: () => void;
}

/** Where you are in the app. Not persisted and not on the server: opening a
 *  share link should land everyone on the overview, not wherever the last
 *  person happened to stop. */
export const useUiStore = create<UiState>((set) => ({
  selectedDay: null,
  view: "route",
  // Relief is the whole point at Verdon and Turini, so topo leads.
  basemap: "topo",
  showCamps: true,
  editing: false,

  selectDay: (n) => set({ selectedDay: n }),
  toggleDay: (n) => set((s) => ({ selectedDay: s.selectedDay === n ? null : n })),
  setView: (v) => set({ view: v }),
  setBasemap: (b) => set({ basemap: b }),
  toggleCamps: () => set((s) => ({ showCamps: !s.showCamps })),
  setEditing: (on) => set({ editing: on }),
  toggleEditing: () => set((s) => ({ editing: !s.editing })),
  reset: () => set({ selectedDay: null, view: "route", editing: false }),
}));
