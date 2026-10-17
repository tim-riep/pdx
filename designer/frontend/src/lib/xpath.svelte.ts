// Zustand des XPath-Editors. Feld im Inspektor und Tab XPath bearbeiten
// denselben Entwurf: das zuletzt fokussierte XPath-Attribut eines Elements.
// Der Entwurf ist nur Eingabe, ins Modell kommt er als Befehl.

import { TemplateAPI, type Completion, type XPathInfo } from "./api";
import { apply } from "./template.svelte";
import { propLabel, t } from "./i18n";

export const xp = $state({
  ref: "",
  prop: "",
  draft: "",
  // info gehört zum Ausdruck infoExpr, ältere Antworten werden verworfen.
  info: null as XPathInfo | null,
  infoExpr: "",
  // caret ist die letzte Cursorstelle im Entwurf, für Einfügen aus Listen.
  caret: 0,
});

export const isActive = (ref: string, prop: string) => xp.ref === ref && xp.prop === prop;

// edit macht ein Attribut zum bearbeiteten Ausdruck.
export function edit(ref: string, prop: string, value: string) {
  if (isActive(ref, prop)) return;
  xp.ref = ref;
  xp.prop = prop;
  xp.draft = value;
  xp.caret = value.length;
  xp.info = null;
  check(0);
}

export function setDraft(v: string) {
  xp.draft = v;
  check();
}

let timer: ReturnType<typeof setTimeout> | undefined;
let seq = 0;

// check prüft den Entwurf nach kurzer Pause beim Tippen.
export function check(delay = 120) {
  clearTimeout(timer);
  const s = ++seq;
  timer = setTimeout(async () => {
    const expr = xp.draft;
    try {
      const info = await TemplateAPI.CheckXPath({ ref: xp.ref, expr, cursor: 0 });
      if (s === seq) {
        xp.info = info;
        xp.infoExpr = expr;
      }
    } catch {
      if (s === seq) xp.info = null;
    }
  }, delay);
}

let cseq = 0;

// complete liefert Vorschläge, veraltete Antworten ergeben null.
export async function complete(ref: string, expr: string, cursor: number): Promise<Completion | null> {
  const s = ++cseq;
  try {
    const c = await TemplateAPI.CompleteXPath({ ref, expr, cursor });
    return s === cseq ? c : null;
  } catch {
    return null;
  }
}

// commit schreibt den Entwurf ins Modell, wenn er sich vom Wert unterscheidet.
export async function commit(current: string): Promise<boolean> {
  const value = xp.draft.trim();
  if (value === current) return true;
  return apply(t("cmd.setAttr", { prop: propLabel(xp.prop) }), { op: "setAttr", ref: xp.ref, name: xp.prop, value });
}

// insert fügt Text an der letzten Cursorstelle ein.
export function insert(text: string) {
  const at = Math.min(xp.caret, xp.draft.length);
  setDraft(xp.draft.slice(0, at) + text + xp.draft.slice(at));
  xp.caret = at + text.length;
}
