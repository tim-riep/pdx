// Befunde der laufenden Prüfung und des letzten Satzlaufs als Lesecache.

import { EVENT, ValidationAPI, on, type ProjectState, type Report } from "./api";
import { render } from "./render.svelte";

export type Entry = { level: string; source: string; ref: string; file: string; msg: string };

export const problems = $state({ report: null as Report | null });

on<Report>(EVENT.problems, (r) => {
  problems.report = r;
});

const load = () =>
  ValidationAPI.Problems()
    .then((r) => (problems.report = r))
    .catch(() => (problems.report = null));
load();

let doc = "";
on<ProjectState>(EVENT.project, (s) => {
  if (!s.open) {
    problems.report = null;
    doc = "";
  } else if (s.doc !== doc) {
    doc = s.doc;
    load();
  }
});

// entries vereint Prüfung und Satz, Fehler zuerst. Doppelte Befunde (der
// Compiler meldet sich in beiden) erscheinen einmal.
export function entries(): Entry[] {
  const out: Entry[] = [];
  const seen = new Set<string>();
  const add = (e: Entry) => {
    const k = `${e.ref}\u0000${e.msg}`;
    if (seen.has(k)) return;
    seen.add(k);
    out.push(e);
  };
  for (const p of problems.report?.problems ?? []) add({ level: p.level, source: p.source, ref: p.ref, file: p.file, msg: p.msg });
  for (const p of render.manifest?.problems ?? []) add({ level: p.level, source: "layout", ref: p.id, file: "", msg: p.msg });
  return out.sort((a, b) => Number(b.level === "Fehler") - Number(a.level === "Fehler"));
}
