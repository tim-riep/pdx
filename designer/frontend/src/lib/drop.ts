// Drop target for dragged data nodes. The geometry comes from the
// areas of the engine, nothing is laid out: only the receiving
// element, the position among its children and, for
// positioned parents, the location in millimeters are searched.

import type { ElementType, ViewNode } from "./api";
import type { HitIndex } from "./canvas/hit";
import type { Area, Page, Rect } from "./canvas/model";

export interface Target {
  parent: string;
  index: number;
  x: number; // mm relative to the parent element, only for positioned parents
  y: number;
  positioned: boolean;
  // marker is the insertion mark (flowing) or the drop point (positioned)
  // in points on the page.
  marker: { x: number; y: number; w: number };
}

const PT_PER_MM = 72 / 25.4;

export interface Model {
  node(ref: string): ViewNode | undefined;
  type(n: ViewNode): ElementType | undefined;
  body(): ViewNode | undefined;
}

// accepts reports whether n accepts a child element with the name child.
function accepts(m: Model, n: ViewNode, child: string): boolean {
  return !!m.type(n)?.children?.some((c) => c?.name === child);
}

function positioned(n: ViewNode): boolean {
  if (n.type === "pageMaster/header" || n.type === "pageMaster/footer") return true;
  return n.name === "subform" && n.attrs.layout === "positioned";
}

const inside = (r: Rect, x: number, y: number) => x >= r[0] && x <= r[0] + r[2] && y >= r[1] && y <= r[1] + r[3];

// dropTarget searches the element under the point that accepts a child with the name
// child (for example field, text, subform or table).
export function dropTarget(m: Model, page: Page, index: HitIndex, x: number, y: number, child = "field"): Target | null {
  const hit = index.at(x, y);
  let n = hit ? m.node(hit.id) : undefined;
  while (n && !accepts(m, n, child)) n = m.node(n.parent);
  n ??= m.body();
  if (!n) return null;
  const container = n;
  const areas = page.els.filter((a) => a.id === container.ref);
  const area: Area | undefined = areas.find((a) => inside(a.r, x, y)) ?? areas[0];
  if (positioned(container)) {
    const ox = area ? area.r[0] : 0, oy = area ? area.r[1] : 0;
    return {
      parent: container.ref, index: -1, positioned: true,
      x: Math.round(((x - ox) / PT_PER_MM) * 2) / 2,
      y: Math.round(((y - oy) / PT_PER_MM) * 2) / 2,
      marker: { x, y, w: 0 },
    };
  }
  // Flowing: before the first child whose center lies below the mouse pointer.
  const kids = container.children.map((c, i) => ({ ref: c?.ref, i })).filter((c) => c.ref);
  let idx = -1;
  let marker = area ? { x: area.r[0], y: area.r[1] + area.r[3], w: area.r[2] } : { x, y, w: 120 };
  let lastBottom = -1;
  for (const k of kids) {
    const a = page.els.find((e) => e.id === k.ref);
    if (!a) continue;
    if (y < a.r[1] + a.r[3] / 2) {
      idx = k.i;
      marker = { x: area ? area.r[0] : a.r[0], y: a.r[1], w: area ? area.r[2] : a.r[2] };
      break;
    }
    lastBottom = a.r[1] + a.r[3];
  }
  if (idx === -1 && lastBottom >= 0) marker = { ...marker, y: lastBottom };
  return { parent: container.ref, index: idx, positioned: false, x: 0, y: 0, marker };
}
