// Oberflächentexte aus einer Ressourcendatei je Sprache. Platzhalter in
// geschweiften Klammern werden ersetzt, etwa "Seite {n}".

import de from "../i18n/de.json";

export type Key = keyof typeof de;

const texts: Record<string, Record<string, string>> = { de };
let lang = "de";

export function setLang(l: string): void {
  if (texts[l]) lang = l;
}

export function t(key: Key, params: Record<string, string | number> = {}): string {
  const s = texts[lang][key] ?? texts.de[key] ?? key;
  return s.replace(/\{(\w+)\}/g, (_, k) => String(params[k] ?? `{${k}}`));
}
