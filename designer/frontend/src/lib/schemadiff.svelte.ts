// Flow of a schema change: choose a new XSD, show the differences,
// apply with the bindings switched over, then a task list.

import { AppAPI, SchemaAPI, message, type SchemaChange, type SchemaDiff } from "./api";
import { t } from "./i18n";
import { ui } from "./project.svelte";
import { tpl } from "./template.svelte";

export type Task = { id: string; text: string; refs: string[]; samples?: boolean };

export const sd = $state({
  open: false,
  path: "",
  diff: null as SchemaDiff | null,
  rename: {} as Record<string, boolean>,
  tasks: [] as Task[],
  done: {} as Record<string, boolean>,
});

async function pick(askPath: (title: string) => Promise<string | null>): Promise<string | null> {
  try {
    const p = await AppAPI.PickFile(t("data.load"), "XML Schema (*.xsd)", "*.xsd");
    return p || null;
  } catch (e) {
    if (!message(e).includes("nicht verfügbar")) {
      ui.error = message(e);
      return null;
    }
    return askPath(t("data.path"));
  }
}

// changeSchema shows the differences to a new XSD.
export async function changeSchema(askPath: (title: string) => Promise<string | null>) {
  const path = await pick(askPath);
  if (!path) return;
  try {
    const diff = await SchemaAPI.Diff(path);
    sd.path = path;
    sd.diff = diff;
    sd.rename = Object.fromEntries(diff.changes.filter((c) => c.kind === "renamed").map((c) => [c.path, true]));
    sd.open = true;
    ui.error = "";
  } catch (e) {
    ui.error = message(e);
  }
}

function taskText(c: SchemaChange): string {
  if (c.kind === "renamed") return t("schemadiff.taskRenamed", { path: c.path, to: c.to });
  if (c.kind === "changed") return t("schemadiff.taskChanged", { path: c.path, detail: c.detail });
  return t("schemadiff.taskRemoved", { path: c.path });
}

export async function applyChange() {
  const diff = sd.diff;
  if (!diff) return;
  const renames = diff.changes.filter((c) => c.kind === "renamed" && sd.rename[c.path]).map((c) => ({ from: c.path, to: c.to }));
  try {
    const res = await SchemaAPI.Replace(sd.path, renames);
    tpl.history = res.history;
    ui.error = "";
  } catch (e) {
    ui.error = message(e);
    return;
  }
  // What remains are nodes with affected elements that are not switched over.
  sd.tasks = diff.changes
    .filter((c) => c.refs.length > 0 && !(c.kind === "renamed" && sd.rename[c.path]) && c.kind !== "added")
    .map((c) => ({ id: `${c.kind}:${c.path}`, text: taskText(c), refs: c.refs }));
  sd.tasks.push({ id: "samples", text: t("schemadiff.taskSamples"), refs: [], samples: true });
  sd.done = {};
  sd.open = false;
}

export function cancelChange() {
  sd.open = false;
  sd.diff = null;
}
