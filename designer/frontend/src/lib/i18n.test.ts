import { describe, expect, it } from "vitest";
import { t } from "./i18n";

describe("t", () => {
  it("ersetzt Platzhalter", () => {
    expect(t("status.version", { version: "1", engine: "2" })).toBe("Designer 1, Engine 2");
  });
  it("lässt fehlende Platzhalter stehen", () => {
    expect(t("status.version", { version: "1" })).toBe("Designer 1, Engine {engine}");
  });
});
