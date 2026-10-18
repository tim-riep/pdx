// Einheit und Hilfen der Ansicht: mm oder pt für Lineale, Abstände und neue
// Werte, Lineale und Seitenübersicht ein oder aus. Gilt je Betrachter und
// bleibt im Browser gespeichert, die Vorlage ändert sich dadurch nicht.

import { PT_PER_MM, fmtMM } from "./geometry";

type Prefs = { unit: "mm" | "pt"; rulers: boolean; pages: boolean };

function load(): Prefs {
  const d: Prefs = { unit: "mm", rulers: true, pages: false };
  try {
    return { ...d, ...JSON.parse(localStorage.getItem("pdx.view") ?? "{}") };
  } catch {
    return d;
  }
}

export const view = $state(load());

$effect.root(() => {
  $effect(() => {
    const s = JSON.stringify({ unit: view.unit, rulers: view.rulers, pages: view.pages });
    try {
      localStorage.setItem("pdx.view", s);
    } catch {
      // ohne Speicher gilt die Wahl nur bis zum Neuladen
    }
  });
});

// fmtLen schreibt Millimeter in der gewählten Einheit, pt auf halbe Punkte.
export function fmtLen(mm: number): string {
  if (view.unit === "pt") {
    const pt = Math.round(mm * PT_PER_MM * 2) / 2;
    return `${Number.isInteger(pt) ? pt.toFixed(0) : pt.toFixed(1)}pt`;
  }
  return fmtMM(mm);
}

// step ist ein Schritt der Pfeiltasten in Millimetern: 1 mm oder 1 pt.
export const step = (n: number) => (view.unit === "pt" ? n / PT_PER_MM : n);
