// Hit test: which area lies under a point of the page? The smallest
// area wins, so inner elements come before their containers.

import type { Area, Page } from "./model";

const cell = 32; // grid size in points

export class HitIndex {
  private grid = new Map<number, Area[]>();
  private cols: number;

  constructor(page: Page) {
    this.cols = Math.ceil(page.w / cell) + 1;
    for (const e of page.els) {
      const [x, y, w, h] = e.r;
      for (let gy = Math.floor(y / cell); gy <= Math.floor((y + h) / cell); gy++) {
        for (let gx = Math.floor(x / cell); gx <= Math.floor((x + w) / cell); gx++) {
          const k = gy * this.cols + gx;
          let list = this.grid.get(k);
          if (!list) this.grid.set(k, (list = []));
          list.push(e);
        }
      }
    }
  }

  at(x: number, y: number): Area | undefined {
    const list = this.grid.get(Math.floor(y / cell) * this.cols + Math.floor(x / cell));
    let best: Area | undefined;
    let area = Infinity;
    for (const e of list ?? []) {
      const [ex, ey, ew, eh] = e.r;
      if (x >= ex && x <= ex + ew && y >= ey && y <= ey + eh && ew * eh < area) {
        best = e;
        area = ew * eh;
      }
    }
    return best;
  }
}
