import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";

// Die API-Schicht spricht mit Wails, im Test ersetzt sie eine Attrappe.
const api = vi.hoisted(() => ({
  Apply: vi.fn(),
}));
vi.mock("../lib/api", () => ({
  EVENT: { project: "p", render: "r", template: "t", closeRequest: "c" },
  on: () => () => {},
  message: (e: unknown) => String((e as Error).message ?? e),
  TemplateAPI: {
    Apply: api.Apply,
    Snapshot: () => Promise.reject(new Error("keine Vorlage")),
    Catalog: () => Promise.resolve({}),
    History: () => Promise.resolve({ undo: "", redo: "", can: [false, false] }),
  },
  ProjectAPI: { State: () => Promise.resolve({ open: false }) },
  RenderAPI: { Latest: () => Promise.resolve({}), SetView: () => {} },
}));

import Inspector from "./Inspector.svelte";
import { tpl } from "../lib/template.svelte";
import { select } from "../lib/selection.svelte";

afterEach(cleanup);

beforeEach(() => {
  api.Apply.mockReset();
  tpl.catalog = {
    subform: {
      key: "subform", name: "subform", mixed: false, text: false, children: [],
      properties: [
        { name: "id", group: "general", kind: "id", required: true },
        { name: "x", group: "position", kind: "length" },
        { name: "layout", group: "position", kind: "enum", enum: ["positioned", "flowed"], required: true },
        { name: "hyphenate", group: "text", kind: "bool" },
      ],
    },
  };
  tpl.snapshot = {
    rev: 1, root: "t",
    nodes: { box: { ref: "box", name: "subform", type: "subform", parent: "t", attrs: { id: "box", x: "20mm", layout: "positioned" }, children: [] } },
  };
  select("box");
});

describe("Inspector", () => {
  it("zeigt die Eigenschaften gruppiert", () => {
    render(Inspector);
    expect(screen.getByText("Teilformular")).toBeTruthy();
    expect(screen.getByText("Allgemein")).toBeTruthy();
    expect(screen.getByText("Lage und Größe")).toBeTruthy();
    expect((screen.getByLabelText("X") as HTMLInputElement).value).toBe("20mm");
  });

  it("schickt eine Änderung als Befehl", async () => {
    api.Apply.mockResolvedValue({ rev: 2, select: "box", history: { undo: "X ändern", redo: "", can: [true, false] } });
    render(Inspector);
    const x = screen.getByLabelText("X") as HTMLInputElement;
    await fireEvent.input(x, { target: { value: "30mm" } });
    await fireEvent.change(x);
    expect(api.Apply).toHaveBeenCalledWith("X ändern", { op: "setAttr", ref: "box", name: "x", value: "30mm" });
  });

  it("setzt den Wert zurück, wenn der Befehl scheitert", async () => {
    api.Apply.mockRejectedValue(new Error("ungültig"));
    render(Inspector);
    const x = screen.getByLabelText("X") as HTMLInputElement;
    await fireEvent.input(x, { target: { value: "zwanzig" } });
    await fireEvent.change(x);
    await vi.waitFor(() => expect(x.value).toBe("20mm"));
  });

  it("filtert über die Suche", async () => {
    render(Inspector);
    await fireEvent.input(screen.getByRole("searchbox"), { target: { value: "anord" } });
    expect(screen.queryByLabelText("X")).toBeNull();
    expect(screen.getByLabelText("Anordnung*")).toBeTruthy();
  });
});
