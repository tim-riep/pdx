import { describe, expect, it } from "vitest";
import { PT_PER_MM, fmtMM, gaps, snap, toMM } from "./geometry";

describe("Längen", () => {
  it("rechnet Einheiten in Millimeter um", () => {
    expect(toMM("20mm")).toBe(20);
    expect(toMM("2cm")).toBe(20);
    expect(toMM("72pt")).toBeCloseTo(25.4);
    expect(toMM("1in")).toBe(25.4);
    expect(toMM("zwanzig")).toBeNull();
    expect(fmtMM(21.04)).toBe("21mm");
    expect(fmtMM(21.46)).toBe("21.5mm");
  });
});

describe("Einrasten", () => {
  const other: [number, number, number, number] = [100, 100, 50, 20];
  it("rastet an der nächsten Kante ein", () => {
    const s = snap([152, 300, 40, 10], [other], 4);
    expect(s.dx).toBeCloseTo(-2);
    expect(s.guides).toEqual([{ axis: "x", at: 150 }]);
  });
  it("ignoriert ferne Kanten", () => {
    expect(snap([200, 300, 40, 10], [other], 4)).toEqual({ dx: 0, dy: 0, guides: [] });
  });
  it("misst Abstände zu Nachbarn", () => {
    const g = gaps([100, 150, 50, 20], [other]);
    expect(g).toHaveLength(1);
    expect(g[0].mm).toBeCloseTo(30 / PT_PER_MM);
  });
});
