// Oberflächentexte aus einer Ressourcendatei je Sprache. Platzhalter in
// geschweiften Klammern werden ersetzt, etwa "Seite {n}".

import de from "../i18n/de.json";

export type Key = keyof typeof de;

const texts: Record<string, Record<string, string>> = { de };
let lang = "de";

export function setLang(l: string): void {
  if (texts[l]) lang = l;
}

export function t(key: Key | string, params: Record<string, string | number> = {}): string {
  const s = texts[lang][key] ?? (texts.de as Record<string, string>)[key] ?? key;
  return s.replace(/\{(\w+)\}/g, (_, k) => String(params[k] ?? `{${k}}`));
}

// has meldet, ob es einen Text zu key gibt.
export function has(key: string): boolean {
  return key in texts[lang] || key in texts.de;
}

// label liefert den Anzeigenamen eines Elementtyps, etwa "Teilformular".
// Lokale Typen wie table/header haben eigene Texte, sonst gilt der Name.
export function label(type: string, name: string): string {
  if (has(`el.${type}`)) return t(`el.${type}`);
  if (has(`el.${name}`)) return t(`el.${name}`);
  return name;
}

// propLabel liefert den Anzeigenamen einer Eigenschaft.
export function propLabel(name: string): string {
  return has(`prop.${name}`) ? t(`prop.${name}`) : name;
}
