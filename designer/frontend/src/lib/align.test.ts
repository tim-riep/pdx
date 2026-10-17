import { describe, expect, it } from "vitest";
import { align, distribute, type Box } from "./align";

const b = (ref: string, x: number, y: number, w: number, h: number): Box => ({ ref, x, y, w, h });

describe("align", () => {
  it("richtet ein Element am Elternelement aus", () => {
    const one = [b("a", 10, 10, 20, 10)];
    expect(align(one, "left", { w: 100, h: 50 })).toEqual([{ ref: "a", x: 0 }]);
    expect(align(one, "hcenter", { w: 100, h: 50 })).toEqual([{ ref: "a", x: 40 }]);
    expect(align(one, "right", { w: 100, h: 50 })).toEqual([{ ref: "a", x: 80 }]);
    expect(align(one, "bottom", { w: 100, h: 50 })).toEqual([{ ref: "a", y: 40 }]);
    expect(align(one, "vcenter", { w: 100, h: 50 })).toEqual([{ ref: "a", y: 20 }]);
  });
  it("richtet mehrere am gemeinsamen Rahmen aus und lässt passende stehen", () => {
    const two = [b("a", 10, 10, 20, 10), b("b", 40, 30, 10, 5)];
    expect(align(two, "left", { w: 100, h: 50 })).toEqual([{ ref: "b", x: 10 }]);
    expect(align(two, "right", { w: 100, h: 50 })).toEqual([{ ref: "a", x: 30 }]);
    expect(align(two, "top", { w: 100, h: 50 })).toEqual([{ ref: "b", y: 10 }]);
  });
});

describe("distribute", () => {
  it("verteilt mit gleichen Abständen", () => {
    const three = [b("a", 0, 0, 10, 5), b("c", 90, 0, 10, 5), b("b", 20, 0, 20, 5)];
    // Spanne 100, Breiten 40, zwei Lücken zu je 30: b beginnt bei 40.
    expect(distribute(three, "x")).toEqual([{ ref: "b", x: 40 }]);
    expect(distribute(three.slice(0, 2), "x")).toEqual([]);
  });
});
