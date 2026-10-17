// Auswahl und Hover gehören zur Ansicht, nicht zum Modell. Ausgewählt ist
// immer ein Vorlagenelement. instance merkt sich das angeklickte Vorkommen
// (Seite und Nummer unter den Bereichen des Elements auf dieser Seite), damit
// es im Canvas stärker erscheint, auch nachdem neu gesetzt wurde.

export const selection = $state({
  ref: "",
  hover: "",
  instance: null as null | { page: number; k: number },
  // reveal bittet den Canvas, das Element sichtbar zu machen.
  reveal: 0,
});

export function select(ref: string, instance: { page: number; k: number } | null = null, reveal = false) {
  selection.ref = ref;
  selection.instance = instance;
  if (reveal) selection.reveal++;
}

export function clearSelection() {
  selection.ref = "";
  selection.instance = null;
}
