import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";

const api = vi.hoisted(() => ({
  Apply: vi.fn(),
  CheckXPath: vi.fn(),
  CompleteXPath: vi.fn(),
}));
vi.mock("../lib/api", () => ({
  EVENT: { project: "p", render: "r", template: "t", closeRequest: "c" },
  on: () => () => {},
  message: (e: unknown) => String((e as Error).message ?? e),
  TemplateAPI: {
    Apply: api.Apply,
    CheckXPath: api.CheckXPath,
    CompleteXPath: api.CompleteXPath,
    Snapshot: () => Promise.reject(new Error("keine Vorlage")),
    Catalog: () => Promise.resolve({}),
    History: () => Promise.resolve({ undo: "", redo: "", can: [false, false] }),
  },
  ProjectAPI: { State: () => Promise.resolve({ open: false }) },
  RenderAPI: { Latest: () => Promise.resolve({}), SetView: () => {} },
}));

import XPathInput from "./XPathInput.svelte";
import { xp } from "../lib/xpath.svelte";

afterEach(cleanup);

beforeEach(() => {
  for (const f of Object.values(api)) f.mockReset();
  xp.ref = "";
  xp.prop = "";
  api.CheckXPath.mockImplementation(({ expr }: { expr: string }) =>
    Promise.resolve(
      expr === "inv:betrag"
        ? { context: "inv:invoice", data: "", problems: [{ pos: 0, end: 10, message: "inv:betrag gibt es unter inv:invoice nicht" }], target: null, result: null }
        : { context: "inv:invoice", data: "samples/r.xml", problems: [], target: null, result: { error: "", count: 1, kind: "nodes", values: ["42"], truth: "true", noContext: false } },
    ),
  );
  api.CompleteXPath.mockResolvedValue({
    from: 0, to: 6, context: "inv:invoice",
    items: [
      { label: "inv:amount", insert: "inv:amount", kind: "element", type: "decimal" },
      { label: "inv:items", insert: "inv:items", kind: "element", type: "", repeated: false },
    ],
  });
});

function setup() {
  render(XPathInput, { ref: "f", prop: "bind", value: "inv:number", label: "Bindung" });
  return screen.getByLabelText("Bindung") as HTMLInputElement;
}

describe("XPathInput", () => {
  it("schlägt vor und übernimmt mit der Eingabetaste", async () => {
    const input = setup();
    await fireEvent.focus(input);
    input.value = "inv:am";
    input.setSelectionRange(6, 6);
    await fireEvent.input(input);
    await screen.findByRole("listbox");
    expect(api.CompleteXPath).toHaveBeenCalledWith({ ref: "f", expr: "inv:am", cursor: 6 });
    expect(screen.getAllByRole("option").map((o) => o.textContent?.replace(/\s+/g, " ").trim())).toEqual(["inv:amount Dezimalzahl", "inv:items Gruppe"]);
    await fireEvent.keyDown(input, { key: "ArrowDown" });
    await fireEvent.keyDown(input, { key: "ArrowUp" });
    await fireEvent.keyDown(input, { key: "Enter" });
    expect(xp.draft).toBe("inv:amount");
    expect(screen.queryByRole("listbox")).toBeNull();
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("1 Treffer"));
  });

  it("zeigt Befunde und verwirft mit Escape", async () => {
    const input = setup();
    await fireEvent.focus(input);
    input.value = "inv:betrag";
    await fireEvent.input(input);
    await waitFor(() => expect(screen.getByRole("status").textContent).toContain("inv:betrag gibt es unter inv:invoice nicht"));
    // The first Escape closes the suggestions, the second discards.
    await screen.findByRole("listbox");
    await fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(xp.draft).toBe("inv:betrag");
    await fireEvent.keyDown(input, { key: "Escape" });
    expect(xp.draft).toBe("inv:number");
    expect(api.Apply).not.toHaveBeenCalled();
  });

  it("schickt den Ausdruck beim Verlassen als Befehl", async () => {
    api.Apply.mockResolvedValue({ rev: 2, select: "", history: { undo: "", redo: "", can: [true, false] } });
    const input = setup();
    await fireEvent.focus(input);
    input.value = " inv:total ";
    await fireEvent.input(input);
    await fireEvent.blur(input);
    await waitFor(() => expect(api.Apply).toHaveBeenCalled());
    expect(api.Apply.mock.calls[0][1]).toMatchObject({ op: "setAttr", ref: "f", name: "bind", value: "inv:total" });
  });
});
