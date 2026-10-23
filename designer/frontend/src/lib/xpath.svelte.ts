// State of the XPath editor. The field in the inspector and the XPath tab edit
// the same draft: the last focused XPath attribute of an element.
// The draft is input only, it reaches the model as a command.

import { TemplateAPI, type Completion, type XPathInfo } from "./api";
import { apply } from "./template.svelte";
import { propLabel, t } from "./i18n";

export const xp = $state({
  ref: "",
  prop: "",
  draft: "",
  // info belongs to the expression infoExpr, older responses are discarded.
  info: null as XPathInfo | null,
  infoExpr: "",
  // caret is the last cursor position in the draft, for inserting from lists.
  caret: 0,
});

export const isActive = (ref: string, prop: string) => xp.ref === ref && xp.prop === prop;

// edit makes an attribute the edited expression.
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

// check checks the draft after a short pause while typing.
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

// complete returns suggestions, outdated responses yield null.
export async function complete(ref: string, expr: string, cursor: number): Promise<Completion | null> {
  const s = ++cseq;
  try {
    const c = await TemplateAPI.CompleteXPath({ ref, expr, cursor });
    return s === cseq ? c : null;
  } catch {
    return null;
  }
}

// commit writes the draft into the model if it differs from the value.
export async function commit(current: string): Promise<boolean> {
  const value = xp.draft.trim();
  if (value === current) return true;
  return apply(t("cmd.setAttr", { prop: propLabel(xp.prop) }), { op: "setAttr", ref: xp.ref, name: xp.prop, value });
}

// insert inserts text at the last cursor position.
export function insert(text: string) {
  const at = Math.min(xp.caret, xp.draft.length);
  setDraft(xp.draft.slice(0, at) + text + xp.draft.slice(at));
  xp.caret = at + text.length;
}
