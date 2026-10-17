import { describe, expect, it } from "vitest";
import { HitIndex } from "./hit";
import type { Page } from "./model";

const page: Page = {
  n: 1, w: 595, h: 842, draw: [],
  els: [
    { id: "tabelle", r: [50, 100, 400, 300], k: "block" },
    { id: "zelle", r: [60, 110, 80, 20], k: "block" },
    { id: "feld", r: [65, 112, 30, 14], k: "text" },
    { id: "rand", r: [500, 800, 90, 40], k: "block" },
  ],
};

describe("HitIndex", () => {
  const idx = new HitIndex(page);
  it("liefert den kleinsten Bereich unter dem Punkt", () => {
    expect(idx.at(70, 118)?.id).toBe("feld");
    expect(idx.at(130, 118)?.id).toBe("zelle");
    expect(idx.at(300, 300)?.id).toBe("tabelle");
  });
  it("findet Bereiche über Rasterzellen hinweg", () => {
    expect(idx.at(449, 399)?.id).toBe("tabelle");
    expect(idx.at(589, 839)?.id).toBe("rand");
  });
  it("liefert nichts außerhalb aller Bereiche", () => {
    expect(idx.at(10, 10)).toBeUndefined();
    expect(idx.at(470, 500)).toBeUndefined();
  });
});
