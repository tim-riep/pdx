import { describe, expect, it } from "vitest";
import { match } from "./palette.svelte";

describe("match", () => {
  it("verlangt alle Wörter und bevorzugt Wortanfänge", () => {
    expect(match({ label: "Tabelle einfügen" }, "tab ein")).toBeGreaterThan(0);
    expect(match({ label: "Tabelle einfügen" }, "tab xyz")).toBe(0);
    expect(match({ label: "Tabelle einfügen" }, "")).toBe(1);
    const start = match({ label: "Konformität prüfen" }, "konf");
    const inner = match({ label: "PDF/A-3b prüfen, Konformität" }, "konf");
    expect(start).toBeGreaterThan(inner);
  });
  it("sucht auch im Hinweis, etwa der ID", () => {
    expect(match({ label: "Teilformular", hint: "adressfenster" }, "adress")).toBeGreaterThan(0);
  });
});
