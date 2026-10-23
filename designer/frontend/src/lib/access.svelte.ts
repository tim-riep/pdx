// Accessibility analysis: structure tree in reading order and
// findings of the pre-check. The backend computes without PDF and without veraPDF,
// the panel asks again after every change.

import { EVENT, RenderAPI, message, on, type Access, type ProjectState } from "./api";

export const access = $state({ data: null as Access | null, loading: false, error: "", showOrder: false, showTabs: false });

let seq = 0;

export async function refreshAccess() {
  const mine = ++seq;
  access.loading = true;
  try {
    const a = await RenderAPI.Access();
    if (mine !== seq) return; // a newer request is running
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

// steps returns the template IDs of the elements with own content (text
// or figure) in reading order, each once. The number of a step
// is its position plus one.
export function steps(a: Access | null): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  const walk = (n: Access["tree"]) => {
    if (!n) return;
    if (n.id && !seen.has(n.id) && (n.text || n.type === "Figure")) {
      seen.add(n.id);
      out.push(n.id);
    }
    n.kids.forEach((k) => walk(k));
  };
  walk(a?.tree ?? null);
  return out;
}

// tabStops returns the template IDs of the form fields in the order
// of the structure tree. With /Tabs /S that is the tab order.
export function tabStops(a: Access | null): string[] {
  const out: string[] = [];
  const walk = (n: Access["tree"]) => {
    if (!n) return;
    if (n.type === "Form" && n.id && !out.includes(n.id)) out.push(n.id);
    n.kids.forEach((k) => walk(k));
  };
  walk(a?.tree ?? null);
  return out;
}
