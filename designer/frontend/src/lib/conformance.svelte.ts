// Stand der Konformitätsprüfung. Das Backend prüft im Hintergrund und meldet
// jeden Wechsel.

import { ConformanceAPI, EVENT, message, on, type Conformance, type ProjectState } from "./api";
import { ui } from "./project.svelte";

export const conf = $state({ state: null as Conformance | null });

on<Conformance>(EVENT.conformance, (c) => {
  conf.state = c;
});

let doc = "";
on<ProjectState>(EVENT.project, (s) => {
  if (s.doc !== doc) {
    doc = s.doc;
    conf.state = null; // anderes Dokument, alter Stand gilt nicht
  }
});

export async function checkConformance() {
  try {
    conf.state = await ConformanceAPI.Check();
  } catch (e) {
    ui.error = message(e);
  }
}

export const cancelConformance = () => ConformanceAPI.Cancel();
