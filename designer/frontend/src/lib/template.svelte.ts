// Projection of the template as a read cache. It is never changed in the frontend,
// but fetched from the backend after every revision.

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

// doc identifies the open document, snapDoc the document of the current
// snapshot. A request remembers its document: a response that arrives after
// a document change is stale and is dropped, otherwise the tree would show
// the previous template. Requests are deduplicated per document and revision.
let doc = "";
let snapDoc = "";
let loading = "";

async function refresh(rev: number) {
  const key = `${doc}:${rev}`;
  if ((tpl.snapshot?.rev === rev && snapDoc === doc) || loading === key) return;
  loading = key;
  const forDoc = doc;
  let snap: Snapshot | null;
  try {
    snap = await TemplateAPI.Snapshot();
  } catch {
    snap = null;
  }
  if (loading === key) loading = "";
  if (forDoc !== doc) return;
  tpl.snapshot = snap;
  snapDoc = forDoc;
}

on<ProjectState>(EVENT.project, (s) => {
  if (!s.open) {
    tpl.snapshot = null;
    doc = "";
    snapDoc = "";
  } else if (s.doc !== doc || tpl.snapshot === null) {
    doc = s.doc;
    tpl.snapshot = null;
    snapDoc = "";
    refresh(s.rev);
    TemplateAPI.History().then((h) => (tpl.history = h));
  }
});

// Patches keep the read cache current. If the revision does not match, the
// whole state is fetched again.
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

// apply sends a command to the backend. Errors appear in the
// status bar, the template then stays unchanged.
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
TemplateAPI.Catalog().then((c) => {
  tpl.catalog = c as Record<string, ElementType>;
});

export function node(ref: string | undefined | null): ViewNode | undefined {
  return ref ? tpl.snapshot?.nodes[ref] : undefined;
}

export function typeOf(n: ViewNode | undefined): ElementType | undefined {
  return n ? tpl.catalog[n.type] : undefined;
}

// path returns the ancestors of an element from the root down to it.
export function path(ref: string): string[] {
  const out: string[] = [];
  for (let n = node(ref); n; n = node(n.parent)) out.unshift(n.ref);
  return out;
}

// A template can already be open when the UI loads.
TemplateAPI.Snapshot()
  .then((s) => (tpl.snapshot = s))
  .catch(() => (tpl.snapshot = null));

// isPositioned reports an element whose parent positions its children.
export function isPositioned(n: ViewNode | undefined): boolean {
  const p = node(n?.parent);
  if (!p) return false;
  return p.type === "pageMaster/header" || p.type === "pageMaster/footer" || (p.name === "subform" && p.attrs.layout === "positioned");
}
