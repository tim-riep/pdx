// Selection and hover belong to the view, not to the model. The selection is
// always a template element. instance remembers the clicked occurrence
// (page and number among the areas of the element on that page), so
// it appears stronger in the canvas, even after a new layout run.

export const selection = $state({
  ref: "",
  hover: "",
  instance: null as null | { page: number; k: number },
  // reveal asks the canvas to make the element visible.
  reveal: 0,
  // extra are further selected elements (Shift+click) for aligning
  // and distributing, siblings of the first.
  extra: [] as string[],
});

export function toggleExtra(ref: string) {
  const i = selection.extra.indexOf(ref);
  if (i >= 0) selection.extra.splice(i, 1);
  else selection.extra.push(ref);
}

export function select(ref: string, instance: { page: number; k: number } | null = null, reveal = false) {
  selection.ref = ref;
  selection.instance = instance;
  selection.extra = [];
  if (reveal) selection.reveal++;
}

export function clearSelection() {
  selection.ref = "";
  selection.instance = null;
  selection.extra = [];
}
