// Kontextmenü für Vorlagenelemente in Hierarchie und Canvas. Das Menü ist
// Ansicht, jede Aktion ist ein Befehl an das Backend.

import { TemplateAPI, message, type Result } from "./api";
import { label, t } from "./i18n";
import { apply, node, tpl } from "./template.svelte";
import { clearSelection, select, selection } from "./selection.svelte";
import { ui } from "./project.svelte";

export type MenuItem = { label: string; action: () => unknown; shortcut?: string; disabled?: boolean; section?: string; testid?: string };

export const menu = $state({ open: false, x: 0, y: 0, items: [] as MenuItem[] });

// seq verwirft Menüs, deren Einträge erst nach dem Schließen eintreffen.
let seq = 0;

export function closeMenu() {
  menu.open = false;
  seq++;
}

async function run(f: () => Promise<Result>) {
  try {
    const res = await f();
    tpl.history = res.history;
    if (res.select) select(res.select, null, true);
    ui.error = "";
  } catch (e) {
    ui.error = message(e);
  }
}

export const duplicate = (ref: string) => run(() => TemplateAPI.Duplicate(ref));
export const moveBy = (ref: string, delta: number) => run(() => TemplateAPI.Move(ref, delta));
export const insertChild = (parent: string, key: string) => run(() => TemplateAPI.InsertChild({ parent, key, index: -1 }));

export function remove(ref: string) {
  const n = node(ref);
  if (!n || !n.parent) return;
  apply(t("cmd.remove", { el: label(n.type, n.name) }), { op: "remove", ref: n.ref });
  if (selection.ref === ref) clearSelection();
}

const mac = typeof navigator !== "undefined" && /Mac/.test(navigator.platform);
const mod = mac ? "⌘" : "Strg+";
const alt = mac ? "⌥" : "Alt+";

// openElementMenu zeigt das Menü für ein Element an einer Bildschirmstelle.
export async function openElementMenu(ref: string, x: number, y: number) {
  const n = node(ref);
  if (!n) return;
  const s = ++seq;
  let insertable: { key: string; name: string }[] = [];
  try {
    insertable = (await TemplateAPI.Insertable(ref)) ?? [];
  } catch {
    insertable = [];
  }
  if (s !== seq) return;
  const items: MenuItem[] = insertable.map((c, i) => ({
    label: label(c.key, c.name),
    action: () => insertChild(ref, c.key),
    section: i === 0 ? t("menu.insert") : undefined,
    testid: `insert-${c.key}`,
  }));
  if (!insertable.length) items.push({ label: t("menu.nothing"), action: () => {}, disabled: true, section: t("menu.insert") });
  const root = !n.parent;
  items.push(
    { label: t("menu.duplicate"), action: () => duplicate(ref), shortcut: `${mod}D`, disabled: root, section: label(n.type, n.name) },
    { label: t("menu.up"), action: () => moveBy(ref, -1), shortcut: `${alt}↑`, disabled: root },
    { label: t("menu.down"), action: () => moveBy(ref, 1), shortcut: `${alt}↓`, disabled: root },
    { label: t("menu.remove"), action: () => remove(ref), shortcut: mac ? "⌫" : "Entf", disabled: root },
  );
  Object.assign(menu, { open: true, x, y, items });
}
