// Command palette: all actions and the search for elements in one place.

export type PaletteItem = { id: string; label: string; group: string; shortcut?: string; hint?: string; run: () => unknown };

export const palette = $state({ open: false });

export const openPalette = () => (palette.open = true);
export const closePalette = () => (palette.open = false);

// match scores an entry: all words of the search must occur,
// matches at the start of a word count more. 0 means no match.
export function match(item: { label: string; hint?: string }, query: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const hay = `${item.label} ${item.hint ?? ""}`.toLowerCase();
  let score = 0;
  for (const w of q.split(/\s+/)) {
    const i = hay.indexOf(w);
    if (i < 0) return 0;
    score += i === 0 ? 3 : /[\s/:(-]/.test(hay[i - 1]) ? 2 : 1;
  }
  return score;
}
