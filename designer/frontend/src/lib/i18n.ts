// UI texts from one resource file per language. Placeholders in
// curly braces are replaced, for example "Seite {n}".

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

// has reports whether there is a text for key.
export function has(key: string): boolean {
  return key in texts[lang] || key in texts.de;
}

// label returns the display name of an element type, for example "Teilformular".
// Local types like table/header have their own texts, otherwise the name applies.
export function label(type: string, name: string): string {
  if (has(`el.${type}`)) return t(`el.${type}`);
  if (has(`el.${name}`)) return t(`el.${name}`);
  return name;
}

// propLabel returns the display name of a property.
export function propLabel(name: string): string {
  return has(`prop.${name}`) ? t(`prop.${name}`) : name;
}

// enumLabel returns the display name of a choice value, for example "rechtsbündig"
// for textAlign end. Without a text the value itself applies.
export function enumLabel(prop: string, value: string): string {
  return has(`enum.${prop}.${value}`) ? t(`enum.${prop}.${value}`) : value;
}

// help returns the explanation of a property or an empty text.
export function help(prop: string): string {
  return has(`help.${prop}`) ? t(`help.${prop}`) : "";
}

// profiles are the offered profile values, common ones first.
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

// profileLabel returns the display name of a profile value.
export function profileLabel(spec: string): string {
  return has(`profile.${spec}`) ? t(`profile.${spec}`) : t("profile.custom", { value: spec });
}

// profileDesc explains a profile value in one sentence.
export function profileDesc(spec: string): string {
  return has(`profile.desc.${spec}`) ? t(`profile.desc.${spec}`) : "";
}

// profileShort returns the standards of a profile value without explanation, for example
// "PDF/A-3a + PDF/UA-1".
export function profileShort(spec: string): string {
  return profileLabel(spec).split(":")[0];
}
