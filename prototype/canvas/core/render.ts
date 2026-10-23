// Draws a page of the area tree into a canvas. Nothing is laid out,
// each glyph sits at the position the engine computed.

import type { Doc, Page } from "./model";

export function drawPage(ctx: CanvasRenderingContext2D, doc: Doc, page: Page, scale: number): void {
  ctx.save();
  ctx.setTransform(scale, 0, 0, scale, 0, 0);
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, page.w, page.h);
  ctx.textBaseline = "alphabetic";
  let font = "";
  for (const it of page.draw) {
    switch (it.k) {
      case "r": {
        const [x, y, w, h] = it.r;
        if (it.bg) {
          ctx.fillStyle = it.bg;
          ctx.fillRect(x, y, w, h);
        }
        if (it.b) {
          // Borders lie inside the rectangle: top, right, bottom, left.
          const [t, r, b, l] = it.b;
          if (t) { ctx.fillStyle = t[1]; ctx.fillRect(x, y, w, t[0]); }
          if (r) { ctx.fillStyle = r[1]; ctx.fillRect(x + w - r[0], y, r[0], h); }
          if (b) { ctx.fillStyle = b[1]; ctx.fillRect(x, y + h - b[0], w, b[0]); }
          if (l) { ctx.fillStyle = l[1]; ctx.fillRect(x, y, l[0], h); }
        }
        break;
      }
      case "t": {
        const f = `${it.s}px ${doc.fonts[it.f].family}`;
        if (f !== font) {
          ctx.font = f;
          font = f;
        }
        ctx.fillStyle = it.c;
        let x = it.x;
        for (let i = 0; i < it.t.length; i++) {
          const s = it.t[i];
          if (s && s !== " ") ctx.fillText(s, x, it.y);
          x += it.a[i];
        }
        break;
      }
      case "l":
        ctx.strokeStyle = it.c;
        ctx.lineWidth = it.w;
        ctx.beginPath();
        ctx.moveTo(it.p[0], it.p[1]);
        ctx.lineTo(it.p[2], it.p[3]);
        ctx.stroke();
        break;
      case "p":
        ctx.fillStyle = it.c;
        for (const [x, y, w, h] of it.r) ctx.fillRect(x, y, w, h);
        break;
    }
  }
  ctx.restore();
}

// PageCache keeps one finished image per page for the current
// magnification. Scrolling and overlays only copy it.
export class PageCache {
  private bitmaps = new Map<number, { scale: number; canvas: HTMLCanvasElement }>();
  constructor(private doc: Doc) {}

  get(page: Page, scale: number): HTMLCanvasElement {
    const hit = this.bitmaps.get(page.n);
    if (hit && hit.scale === scale) return hit.canvas;
    const canvas = hit?.canvas ?? document.createElement("canvas");
    canvas.width = Math.ceil(page.w * scale);
    canvas.height = Math.ceil(page.h * scale);
    drawPage(canvas.getContext("2d")!, this.doc, page, scale);
    this.bitmaps.set(page.n, { scale, canvas });
    return canvas;
  }
}
