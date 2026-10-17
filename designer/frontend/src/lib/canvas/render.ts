// Zeichnet eine Seite aus dem Area Tree in einen Canvas. Es wird nichts
// gesetzt: jede Glyphe steht dort, wo die Engine sie berechnet hat.

import type { Border, Page, Rect } from "./model";

// images liefert ein geladenes Bild zu einem Schlüssel oder undefined.
export type ImageSource = (key: string) => HTMLImageElement | undefined;

export function drawPage(ctx: CanvasRenderingContext2D, page: Page, scale: number, images: ImageSource): void {
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
        if (it.b) borders(ctx, it.r, it.b);
        break;
      }
      case "t": {
        const f = `${it.s}px pdx-${it.f}`;
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
        line(ctx, it.p[0], it.p[1], it.p[2], it.p[3], it.w, it.c, it.st);
        break;
      case "p":
        ctx.fillStyle = it.c;
        for (const [x, y, w, h] of it.r) ctx.fillRect(x, y, w, h);
        break;
      case "i": {
        const img = images(it.i);
        const [x, y, w, h] = it.r;
        if (img?.complete && img.naturalWidth) ctx.drawImage(img, x, y, w, h);
        else {
          ctx.fillStyle = "#e4e6ea";
          ctx.fillRect(x, y, w, h);
        }
        break;
      }
    }
  }
  ctx.restore();
}

// borders zeichnet die Kanten außerhalb des Rechtecks: oben, rechts, unten,
// links, wie im Area Tree.
function borders(ctx: CanvasRenderingContext2D, r: Rect, b: Border[]): void {
  const [x, y, w, h] = r;
  const [t, rt, bt, l] = b;
  const tw = t ? t[0] : 0, rw = rt ? rt[0] : 0, bw = bt ? bt[0] : 0, lw = l ? l[0] : 0;
  if (t) edge(ctx, x - lw, y - tw / 2, x + w + rw, y - tw / 2, t);
  if (bt) edge(ctx, x - lw, y + h + bw / 2, x + w + rw, y + h + bw / 2, bt);
  if (l) edge(ctx, x - lw / 2, y - tw, x - lw / 2, y + h + bw, l);
  if (rt) edge(ctx, x + w + rw / 2, y - tw, x + w + rw / 2, y + h + bw, rt);
}

function edge(ctx: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number, b: [number, string, string]): void {
  line(ctx, x1, y1, x2, y2, b[0], b[1], b[2]);
}

function line(ctx: CanvasRenderingContext2D, x1: number, y1: number, x2: number, y2: number, w: number, c: string, st = ""): void {
  ctx.save();
  ctx.strokeStyle = c;
  if (st === "double") {
    // Zwei Linien je ein Drittel der Breite.
    const t = w / 3;
    const dx = y1 === y2 ? 0 : t, dy = y1 === y2 ? t : 0;
    ctx.lineWidth = t;
    for (const s of [-1, 1]) {
      ctx.beginPath();
      ctx.moveTo(x1 + s * dx, y1 + s * dy);
      ctx.lineTo(x2 + s * dx, y2 + s * dy);
      ctx.stroke();
    }
    ctx.restore();
    return;
  }
  ctx.lineWidth = w;
  if (st === "dashed") ctx.setLineDash([3 * w, 2 * w]);
  if (st === "dotted") {
    ctx.setLineDash([0, 2 * w]);
    ctx.lineCap = "round";
  }
  ctx.beginPath();
  ctx.moveTo(x1, y1);
  ctx.lineTo(x2, y2);
  ctx.stroke();
  ctx.restore();
}
