// Design-Tokens aus dem Lesecache der Vorlage und Befehle dazu.

import { TemplateAPI, message, type TokenOp, type ViewNode } from "./api";
import { node, tpl } from "./template.svelte";
import { select } from "./selection.svelte";
import { ui } from "./project.svelte";

export type Token = { ref: string; id: string; kind: "color" | "length" | "font"; attrs: Record<string, string | undefined> };

function rootChild(name: string): ViewNode | undefined {
  const s = tpl.snapshot;
  if (!s) return undefined;
  for (const c of node(s.root)?.children ?? []) {
    const n = node(c.ref);
    if (n?.name === name) return n;
  }
  return undefined;
}

export function tokens(): Token[] {
  const tk = rootChild("tokens");
  const out: Token[] = [];
  for (const c of tk?.children ?? []) {
    const n = node(c.ref);
    if (n && (n.name === "color" || n.name === "length" || n.name === "font")) out.push({ ref: n.ref, id: n.attrs.id ?? "", kind: n.name, attrs: n.attrs });
  }
  return out;
}

export function styles(): ViewNode[] {
  const st = rootChild("styles");
  return (st?.children ?? []).map((c) => node(c.ref)).filter((n): n is ViewNode => !!n && n.name === "style");
}

// resolveColor liefert die Farbe hinter einem Wert, Tokens auch über Ketten.
export function resolveColor(v: string, list = tokens(), depth = 0): string {
  if (!v.startsWith("$") || depth > 8) return v;
  const t = list.find((x) => x.id === v.slice(1) && x.kind === "color");
  return t ? resolveColor(t.attrs.value ?? "", list, depth + 1) : "";
}

// hex6 bringt eine Farbe in die Form #rrggbb für input type=color.
export function hex6(c: string): string {
  if (/^#[0-9a-f]{6}$/i.test(c)) return c.toLowerCase();
  if (/^#[0-9a-f]{3}$/i.test(c)) return ("#" + [...c.slice(1)].map((x) => x + x).join("")).toLowerCase();
  return "#000000";
}

export async function tokenOp(op: Partial<TokenOp> & { op: string }): Promise<boolean> {
  try {
    const res = await TemplateAPI.Tokens({ kind: "", id: "", newId: "", ...op } as TokenOp);
    tpl.history = res.history;
    ui.error = "";
    return true;
  } catch (e) {
    ui.error = message(e);
    return false;
  }
}

export async function addStyle(parent = ""): Promise<string> {
  try {
    const res = await TemplateAPI.AddStyle(parent);
    tpl.history = res.history;
    if (res.select) select(res.select, null, true);
    ui.error = "";
    return res.select;
  } catch (e) {
    ui.error = message(e);
    return "";
  }
}
