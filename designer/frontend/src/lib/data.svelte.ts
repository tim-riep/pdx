// Datenbaum des Daten-XSD als Lesecache, neu geholt nach jeder Änderung der
// Vorlage (das Backend cacht das aufbereitete Schema).

import { EVENT, SchemaAPI, message, on, type DataNode, type DataSchema, type ProjectState } from "./api";
import { ui } from "./project.svelte";
import { select } from "./selection.svelte";
import { t } from "./i18n";

export const data = $state({ schema: null as DataSchema | null });

let seq = 0;
async function refresh() {
  const n = ++seq;
  try {
    const s = await SchemaAPI.Tree();
    if (n === seq) data.schema = s;
  } catch {
    if (n === seq) data.schema = null;
  }
}

on<ProjectState>(EVENT.project, (s) => (s.open ? refresh() : (data.schema = null)));
refresh();

// MIME-Typ für das Ziehen von Datenknoten.
export const DATA_MIME = "application/x-pdx-data";

export async function loadSchema(askPath: (title: string) => Promise<string | null>) {
  try {
    const res = await SchemaAPI.ImportDialog("");
    if (res.rev) select(res.select);
  } catch (e) {
    if (!message(e).includes("nicht verfügbar")) {
      ui.error = message(e);
      return;
    }
    const path = await askPath(t("data.path"));
    if (!path) return;
    try {
      await SchemaAPI.Import(path, "");
      ui.error = "";
    } catch (e2) {
      ui.error = message(e2);
    }
  }
}

// typeKey liefert die Art eines Datenknotens für Symbol und Beschriftung.
export function typeKey(n: DataNode): string {
  if (!n.type) return "complex";
  if (["decimal", "double", "float"].includes(n.type)) return "decimal";
  if (n.type.toLowerCase().includes("int") || ["long", "short", "byte"].includes(n.type)) return "integer";
  if (["date", "dateTime", "boolean"].includes(n.type)) return n.type;
  return "string";
}

export function cardinality(n: DataNode): string {
  const max = n.max === -1 ? "*" : String(n.max);
  return `${n.min}..${max}`;
}

// kids liefert die Kinder ohne Null-Einträge (Go-Zeigerlisten).
export function kids(n: DataNode | null | undefined): DataNode[] {
  return (n?.children ?? []).filter((c): c is DataNode => !!c);
}

export function roots(): DataNode[] {
  return (data.schema?.roots ?? []).filter((c): c is DataNode => !!c);
}
