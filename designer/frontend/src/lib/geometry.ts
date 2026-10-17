// Längen und Einrasten für Verschieben und Größe ändern. Alle Rechtecke in
// Punkt auf der Seite, Vorlagenwerte in Millimetern.

import type { Rect } from "./canvas/model";

export const PT_PER_MM = 72 / 25.4;

const units: Record<string, number> = { mm: 1, cm: 10, in: 25.4, pt: 1 / PT_PER_MM, pc: 12 / PT_PER_MM, px: 0.75 / PT_PER_MM };

// toMM liest eine Länge wie "20mm" oder "12pt" in Millimetern.
export function toMM(v: string | undefined): number | null {
  if (!v) return null;
  const m = /^(-?[0-9]+(?:\.[0-9]+)?)(mm|cm|in|pt|pc|px)?$/.exec(v.trim());
  if (!m) return null;
  return parseFloat(m[1]) * (units[m[2] ?? "pt"] ?? 1);
}

// fmtMM schreibt Millimeter mit höchstens einer Nachkommastelle.
export function fmtMM(v: number): string {
  const r = Math.round(v * 10) / 10;
  return `${Number.isInteger(r) ? r.toFixed(0) : r.toFixed(1)}mm`;
}

export interface Guide {
  axis: "x" | "y";
  at: number; // pt
}

export interface Snap {
  dx: number;
  dy: number;
  guides: Guide[];
}

// edges liefert Anfang, Mitte und Ende eines Rechtecks je Achse.
function edges(r: Rect): { x: number[]; y: number[] } {
  return { x: [r[0], r[0] + r[2] / 2, r[0] + r[2]], y: [r[1], r[1] + r[3] / 2, r[1] + r[3]] };
}

// snap verschiebt ein Rechteck auf die nächste Hilfslinie in der Nähe:
// Kanten und Mitten der Nachbarn und des Elternelements.
export function snap(r: Rect, others: Rect[], threshold: number): Snap {
  const me = edges(r);
  let best = { x: { d: threshold + 1, at: 0, shift: 0 }, y: { d: threshold + 1, at: 0, shift: 0 } };
  for (const o of others) {
    const e = edges(o);
    for (const axis of ["x", "y"] as const) {
      for (const a of me[axis]) {
        for (const b of e[axis]) {
          const d = Math.abs(a - b);
          if (d < best[axis].d) best[axis] = { d, at: b, shift: b - a };
        }
      }
    }
  }
  const guides: Guide[] = [];
  let dx = 0, dy = 0;
  if (best.x.d <= threshold) {
    dx = best.x.shift;
    guides.push({ axis: "x", at: best.x.at });
  }
  if (best.y.d <= threshold) {
    dy = best.y.shift;
    guides.push({ axis: "y", at: best.y.at });
  }
  return { dx, dy, guides };
}

// gaps liefert die Abstände zu den nächsten Nachbarn links, rechts, oben und
// unten, soweit sie sich in der anderen Achse überlappen.
export function gaps(r: Rect, others: Rect[]): { from: [number, number]; to: [number, number]; mm: number }[] {
  const out: { from: [number, number]; to: [number, number]; mm: number }[] = [];
  const [x, y, w, h] = r;
  let left: Rect | null = null, right: Rect | null = null, up: Rect | null = null, down: Rect | null = null;
  for (const o of others) {
    const overlapY = o[1] < y + h && o[1] + o[3] > y;
    const overlapX = o[0] < x + w && o[0] + o[2] > x;
    if (overlapY && o[0] + o[2] <= x && (!left || o[0] + o[2] > left[0] + left[2])) left = o;
    if (overlapY && o[0] >= x + w && (!right || o[0] < right[0])) right = o;
    if (overlapX && o[1] + o[3] <= y && (!up || o[1] + o[3] > up[1] + up[3])) up = o;
    if (overlapX && o[1] >= y + h && (!down || o[1] < down[1])) down = o;
  }
  const cy = y + h / 2, cx = x + w / 2;
  if (left) out.push({ from: [left[0] + left[2], cy], to: [x, cy], mm: (x - left[0] - left[2]) / PT_PER_MM });
  if (right) out.push({ from: [x + w, cy], to: [right[0], cy], mm: (right[0] - x - w) / PT_PER_MM });
  if (up) out.push({ from: [cx, up[1] + up[3]], to: [cx, y], mm: (y - up[1] - up[3]) / PT_PER_MM });
  if (down) out.push({ from: [cx, y + h], to: [cx, down[1]], mm: (down[1] - y - h) / PT_PER_MM });
  return out;
}
