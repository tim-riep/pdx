// Projektion der Vorlage als Lesecache. Sie wird nie im Frontend geändert,
// sondern nach jeder Revision aus dem Backend geholt.

import {
  EVENT,
  TemplateAPI,
  message,
  on,
  type Command,
  type ElementType,
  type History,
  type Patch,
  type ProjectState,
  type Snapshot,
  type ViewNode,
} from "./api";
import { select } from "./selection.svelte";
import { ui } from "./project.svelte";

export const tpl = $state({
  snapshot: null as Snapshot | null,
  catalog: {} as Record<string, ElementType>,
  history: { undo: "", redo: "", can: [false, false] } as History,
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

let doc = "";

on<ProjectState>(EVENT.project, (s) => {
  if (!s.open) {
    tpl.snapshot = null;
    doc = "";
  } else if (s.doc !== doc || tpl.snapshot === null) {
    doc = s.doc;
    tpl.snapshot = null;
    refresh(s.rev);
    TemplateAPI.History().then((h) => (tpl.history = h));
  }
});

// Patches halten den Lesecache aktuell. Passt die Revision nicht, wird der
// ganze Stand neu geholt.
on<Patch>(EVENT.template, (p) => {
  const s = tpl.snapshot;
  if (!s || p.from !== s.rev) {
    refresh(p.rev);
    return;
  }
  const nodes = { ...s.nodes };
  for (const r of p.removed) delete nodes[r];
  for (const n of p.set) nodes[n.ref] = n;
  tpl.snapshot = { rev: p.rev, root: p.root, nodes };
});

// apply schickt einen Befehl an das Backend. Fehler erscheinen in der
// Statusleiste, die Vorlage bleibt dann unverändert.
export async function apply(label: string, cmd: Partial<Command> & { op: string }): Promise<boolean> {
  try {
    const res = await TemplateAPI.Apply(label, cmd as Command);
    tpl.history = res.history;
    if (res.select) select(res.select);
    ui.error = "";
    return true;
  } catch (e) {
    ui.error = message(e);
    return false;
  }
}

export async function undo() {
  try {
    tpl.history = (await TemplateAPI.Undo()).history;
  } catch (e) {
    ui.error = message(e);
  }
}

export async function redo() {
  try {
    tpl.history = (await TemplateAPI.Redo()).history;
  } catch (e) {
    ui.error = message(e);
  }
}
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
