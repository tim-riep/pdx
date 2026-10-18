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

// enumLabel liefert den Anzeigenamen eines Auswahlwerts, etwa "rechtsbündig"
// für textAlign end. Ohne Text gilt der Wert selbst.
export function enumLabel(prop: string, value: string): string {
  return has(`enum.${prop}.${value}`) ? t(`enum.${prop}.${value}`) : value;
}

// help liefert die Erklärung einer Eigenschaft oder einen leeren Text.
export function help(prop: string): string {
  return has(`help.${prop}`) ? t(`help.${prop}`) : "";
}

// profiles sind die angebotenen Profilangaben, häufige zuerst.
export const profiles = [
  "pdfa-3b",
  "pdfa-3u",
  "pdfa-3a,pdfua-1",
  "pdfa-4f,pdfua-2",
  "pdfa-4,pdfua-2",
  "pdfa-4f",
  "pdfa-4",
  "pdfa-2b",
  "pdfa-2u",
  "pdf-1.7",
  "pdf-2.0",
];

// profileLabel liefert den Anzeigenamen einer Profilangabe.
export function profileLabel(spec: string): string {
  return has(`profile.${spec}`) ? t(`profile.${spec}`) : t("profile.custom", { value: spec });
}

// profileDesc erklärt eine Profilangabe in einem Satz.
export function profileDesc(spec: string): string {
  return has(`profile.desc.${spec}`) ? t(`profile.desc.${spec}`) : "";
}

// profileShort liefert die Normen einer Profilangabe ohne Erklärung, etwa
// "PDF/A-3a + PDF/UA-1".
export function profileShort(spec: string): string {
  return profileLabel(spec).split(":")[0];
}
