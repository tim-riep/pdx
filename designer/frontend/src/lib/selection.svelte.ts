// Auswahl und Hover gehören zur Ansicht, nicht zum Modell. Ausgewählt ist
// immer ein Vorlagenelement. instance merkt sich das angeklickte Vorkommen,
// damit es im Canvas stärker erscheint.

import type { Rect } from "./canvas/model";

export const selection = $state({
  ref: "",
  hover: "",
  instance: null as null | { page: number; r: Rect },
  // reveal bittet den Canvas, das Element sichtbar zu machen.
  reveal: 0,
});

export function select(ref: string, instance: { page: number; r: Rect } | null = null, reveal = false) {
  selection.ref = ref;
  selection.instance = instance;
  if (reveal) selection.reveal++;
}

export function clearSelection() {
  selection.ref = "";
  selection.instance = null;
}
