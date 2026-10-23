// Inserting building blocks from the bar above the canvas.

import { AppAPI, TemplateAPI, message } from "./api";
import { askPath, ui } from "./project.svelte";
import { select, selection } from "./selection.svelte";
import { node, tpl, typeOf } from "./template.svelte";
import { t } from "./i18n";

export const ELEMENT_MIME = "application/x-pdx-element";

export const kinds = ["text", "positioned", "flowed", "line", "rect", "image", "barcode", "pageNumber", "table",
  "textField", "checkBox", "radioGroup", "comboBox", "listBox", "signatureField"] as const;
export type Kind = (typeof kinds)[number];

// elementName is the element name of a building block in the template.
export const elementName: Record<Kind, string> = {
  text: "text", positioned: "subform", flowed: "subform", line: "line", rect: "rect",
  image: "image", barcode: "barcode", pageNumber: "pageNumber", table: "table",
  textField: "textField", checkBox: "checkBox", radioGroup: "radioGroup", comboBox: "comboBox",
  listBox: "listBox", signatureField: "signatureField",
};

// acceptsContent reports whether an element accepts content such as fields.
export function acceptsContent(ref: string): boolean {
  const n = node(ref);
  return !!typeOf(n)?.children?.some((c) => c?.name === "field");
}

// imageFile asks for an image file, in the browser via the path prompt.
async function imageFile(): Promise<string | null> {
  try {
    const path = await AppAPI.PickFile(t("insert.image"), t("insert.imageFilter"), "*.png;*.jpg;*.jpeg");
    return path || null;
  } catch (e) {
    if (!message(e).includes("nicht verfügbar")) {
      ui.error = message(e);
      return null;
    }
    return askPath(t("insert.imagePath"));
  }
}

export async function insertElement(kind: Kind, parent: string, index: number, x = 10, y = 10) {
  let file = "";
  if (kind === "image") {
    const f = await imageFile();
    if (!f) return;
    file = f;
  }
  try {
    const res = await TemplateAPI.InsertElement({ kind, parent, index, x, y, file });
    tpl.history = res.history;
    if (res.select) select(res.select);
    ui.error = "";
  } catch (e) {
    ui.error = message(e);
  }
}

// accepts reports whether an element allows the building block as a child. A text
// accepts fields, but no further text.
function accepts(ref: string, kind: Kind): boolean {
  return !!typeOf(node(ref))?.children?.some((c) => c?.name === elementName[kind]);
}

// insertAtSelection inserts into the selection if it allows the building block,
// otherwise after the selection, without a selection at the end of the content.
export function insertAtSelection(kind: Kind) {
  const sel = node(selection.ref);
  if (sel && accepts(sel.ref, kind)) return insertElement(kind, sel.ref, -1);
  if (sel?.parent && accepts(sel.parent, kind)) {
    const parent = node(sel.parent)!;
    const i = parent.children.findIndex((c) => c?.ref === sel.ref);
    return insertElement(kind, parent.ref, i < 0 ? -1 : i + 1);
  }
  const body = Object.values(tpl.snapshot?.nodes ?? {}).find((n) => n?.name === "body");
  if (body) return insertElement(kind, body.ref, -1);
}
