// Projektion der Vorlage als Lesecache. Sie wird nie im Frontend geändert,
// sondern nach jeder Revision aus dem Backend geholt.

import { EVENT, TemplateAPI, on, type ElementType, type ProjectState, type Snapshot, type ViewNode } from "./api";

export const tpl = $state({
  snapshot: null as Snapshot | null,
  catalog: {} as Record<string, ElementType>,
});

let loading = -1;

async function refresh(rev: number) {
  if (tpl.snapshot?.rev === rev || loading === rev) return;
  loading = rev;
  try {
    tpl.snapshot = await TemplateAPI.Snapshot();
  } catch {
    tpl.snapshot = null;
  }
  loading = -1;
}

on<ProjectState>(EVENT.project, (s) => (s.open ? refresh(s.rev) : (tpl.snapshot = null)));
TemplateAPI.Catalog().then((c) => (tpl.catalog = c as Record<string, ElementType>));

export function node(ref: string | undefined | null): ViewNode | undefined {
  return ref ? tpl.snapshot?.nodes[ref] : undefined;
}

export function typeOf(n: ViewNode | undefined): ElementType | undefined {
  return n ? tpl.catalog[n.type] : undefined;
}

// path liefert die Vorfahren eines Elements von der Wurzel bis zu ihm.
export function path(ref: string): string[] {
  const out: string[] = [];
  for (let n = node(ref); n; n = node(n.parent)) out.unshift(n.ref);
  return out;
}

// Beim Laden der Oberfläche kann schon eine Vorlage offen sein.
TemplateAPI.Snapshot()
  .then((s) => (tpl.snapshot = s))
  .catch(() => (tpl.snapshot = null));
