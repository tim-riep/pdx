// Aligning and distributing positioned elements. All values in
// millimeters relative to the parent element.

export type Box = { ref: string; x: number; y: number; w: number; h: number };
export type AlignKind = "left" | "hcenter" | "right" | "top" | "vcenter" | "bottom";
export type Move = { ref: string; x?: number; y?: number };

// align aligns one element to the parent element, several to their
// common bounding box.
export function align(boxes: Box[], kind: AlignKind, parent: { w: number; h: number }): Move[] {
  if (!boxes.length) return [];
  const frame =
    boxes.length === 1
      ? { x: 0, y: 0, w: parent.w, h: parent.h }
      : (() => {
          const x0 = Math.min(...boxes.map((b) => b.x)), y0 = Math.min(...boxes.map((b) => b.y));
          const x1 = Math.max(...boxes.map((b) => b.x + b.w)), y1 = Math.max(...boxes.map((b) => b.y + b.h));
          return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
        })();
  const out: Move[] = [];
  for (const b of boxes) {
    let m: Move | null = null;
    switch (kind) {
      case "left": m = { ref: b.ref, x: frame.x }; break;
      case "hcenter": m = { ref: b.ref, x: frame.x + (frame.w - b.w) / 2 }; break;
      case "right": m = { ref: b.ref, x: frame.x + frame.w - b.w }; break;
      case "top": m = { ref: b.ref, y: frame.y }; break;
      case "vcenter": m = { ref: b.ref, y: frame.y + (frame.h - b.h) / 2 }; break;
      case "bottom": m = { ref: b.ref, y: frame.y + frame.h - b.h }; break;
    }
    const changed = (m.x !== undefined && Math.abs(m.x - b.x) > 0.05) || (m.y !== undefined && Math.abs(m.y - b.y) > 0.05);
    if (changed) out.push(m);
  }
  return out;
}

// distribute spreads three or more elements with equal spacing
// between the first and the last.
export function distribute(boxes: Box[], axis: "x" | "y"): Move[] {
  if (boxes.length < 3) return [];
  const pos = (b: Box) => (axis === "x" ? b.x : b.y);
  const size = (b: Box) => (axis === "x" ? b.w : b.h);
  const sorted = [...boxes].sort((a, b) => pos(a) - pos(b));
  const first = sorted[0], last = sorted[sorted.length - 1];
  const span = pos(last) + size(last) - pos(first);
  const total = sorted.reduce((s, b) => s + size(b), 0);
  const gap = (span - total) / (sorted.length - 1);
  const out: Move[] = [];
  let at = pos(first) + size(first) + gap;
  for (const b of sorted.slice(1, -1)) {
    if (Math.abs(at - pos(b)) > 0.05) out.push(axis === "x" ? { ref: b.ref, x: at } : { ref: b.ref, y: at });
    at += size(b) + gap;
  }
  return out;
}
