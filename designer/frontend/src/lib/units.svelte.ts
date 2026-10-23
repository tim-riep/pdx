// Unit and aids of the view: mm or pt for rulers, distances and new
// values, rulers and page overview on or off. Applies per viewer and
// stays stored in the browser, the template does not change because of it.

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
      // without storage the choice applies only until reloading
    }
  });
});

// fmtLen writes millimeters in the chosen unit, pt to half points.
export function fmtLen(mm: number): string {
  if (view.unit === "pt") {
    const pt = Math.round(mm * PT_PER_MM * 2) / 2;
    return `${Number.isInteger(pt) ? pt.toFixed(0) : pt.toFixed(1)}pt`;
  }
  return fmtMM(mm);
}

// step is one step of the arrow keys in millimeters: 1 mm or 1 pt.
export const step = (n: number) => (view.unit === "pt" ? n / PT_PER_MM : n);
