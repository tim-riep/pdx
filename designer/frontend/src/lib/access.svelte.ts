// Analyse der Barrierefreiheit: Strukturbaum in Lesereihenfolge und
// Befunde der Vorprüfung. Das Backend rechnet ohne PDF und ohne veraPDF,
// das Panel fragt nach jeder Änderung neu.

import { EVENT, RenderAPI, message, on, type Access, type ProjectState } from "./api";

export const access = $state({ data: null as Access | null, loading: false, error: "" });

let seq = 0;

export async function refreshAccess() {
  const mine = ++seq;
  access.loading = true;
  try {
    const a = await RenderAPI.Access();
    if (mine !== seq) return; // eine neuere Anfrage läuft
    access.data = a;
    access.error = "";
  } catch (e) {
    if (mine === seq) access.error = message(e);
  } finally {
    if (mine === seq) access.loading = false;
  }
}

let doc = "";
on<ProjectState>(EVENT.project, (s) => {
  if (s.doc !== doc) {
    doc = s.doc;
    access.data = null;
  }
});

// order liefert die Vorlagen-IDs in Lesereihenfolge, jede einmal.
export function order(a: Access | null): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const walk = (n: Access["tree"]) => {
    if (!n) return;
    if (n.id && !seen.has(n.id)) {
      seen.add(n.id);
      out.push(n.id);
    }
    n.kids.forEach(walk);
  };
  walk(a?.tree ?? null);
  return out;
}
