import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The API layer talks to Wails, in the test a stand-in replaces it.
const api = vi.hoisted(() => ({
  Settings: vi.fn(),
  SetCheck: vi.fn(),
  Skip: vi.fn(),
  Check: vi.fn(),
  Open: vi.fn(),
}));
vi.mock("./api", () => ({
  message: (e: unknown) => String((e as Error).message ?? e),
  UpdateAPI: api,
}));

const newer = {
  current: "0.3.2",
  latest: "0.4.0",
  url: "https://github.com/tim-riep/pdx/releases/tag/v0.4.0",
  newer: true,
  notesHtml: "<p>Neu</p>",
  publishedAt: "2026-10-24T10:30:00Z",
};

// Every test gets fresh module state (the dialog opens once per start).
async function load() {
  vi.resetModules();
  return import("./update.svelte");
}

const flush = () => new Promise((r) => setTimeout(r, 0));

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
  for (const f of Object.values(api)) f.mockReset();
  api.Settings.mockResolvedValue({ check: true, skipped: "" });
  api.Check.mockResolvedValue(newer);
});
afterEach(() => vi.useRealTimers());

describe("Suche nach neuen Versionen", () => {
  it("öffnet den Dialog einmal je Start, wenn es eine neuere Version gibt", async () => {
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    expect(m.update.info?.latest).toBe("0.4.0");
    expect(m.update.open).toBe(true);
    m.update.open = false;
    vi.advanceTimersByTime(6 * 60 * 60 * 1000);
    await flush();
    expect(api.Check).toHaveBeenCalledTimes(2);
    expect(m.update.open).toBe(false);
  });

  it("öffnet den Dialog für eine übersprungene Version nicht von selbst", async () => {
    api.Settings.mockResolvedValue({ check: true, skipped: "0.4.0" });
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    expect(m.update.info?.newer).toBe(true);
    expect(m.update.open).toBe(false);
  });

  it("fragt ausgeschaltet nicht und vergisst die letzte Antwort", async () => {
    api.Settings.mockResolvedValue({ check: false, skipped: "" });
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    expect(api.Check).not.toHaveBeenCalled();
    expect(m.update.info).toBeNull();
  });

  it("behält bei einem Fehler die letzte gute Antwort", async () => {
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    api.Check.mockResolvedValue({ current: "0.3.2", latest: "", url: "", newer: false, notesHtml: "", publishedAt: "", error: "offline" });
    vi.advanceTimersByTime(6 * 60 * 60 * 1000);
    await flush();
    expect(m.update.info?.latest).toBe("0.4.0");
  });

  it("überspringt die Version und schließt den Dialog", async () => {
    api.Skip.mockResolvedValue({ check: true, skipped: "0.4.0" });
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    await m.skipRelease();
    expect(api.Skip).toHaveBeenCalledWith("0.4.0");
    expect(m.update.open).toBe(false);
    expect(m.update.settings.skipped).toBe("0.4.0");
  });

  it("öffnet die Release-Seite ohne Browser des Systems in einem neuen Tab", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    api.Open.mockRejectedValue(new Error("Dateidialoge sind hier nicht verfügbar"));
    const m = await load();
    await m.startUpdateCheck();
    await flush();
    await m.openRelease();
    expect(api.Open).toHaveBeenCalledWith(newer.url);
    expect(open).toHaveBeenCalledWith(newer.url, "_blank", "noopener");
    api.Open.mockRejectedValue(new Error("nur Release-Seiten von pdx lassen sich öffnen"));
    open.mockClear();
    await m.openRelease("https://example.org/");
    expect(open).not.toHaveBeenCalled();
    expect(m.update.error).toContain("Release-Seiten");
  });
});

describe("sanitizeNotes", () => {
  it("lässt einfache Formatierung und Links stehen", async () => {
    const { sanitizeNotes } = await load();
    const html = '<h2>Neu</h2><ul><li><strong>a</strong> <code>b</code></li></ul><p><a href="https://github.com/tim-riep/pdx">x</a></p>';
    expect(sanitizeNotes(html)).toBe(html);
  });

  it("entfernt Skripte, Bilder, Ereignisattribute und gefährliche Links", async () => {
    const { sanitizeNotes } = await load();
    const out = sanitizeNotes(
      '<p onclick="x()" class="c" style="color:red">Text<script>alert(1)</script><img src="https://t.example/p.gif"></p>' +
        '<a href="javascript:alert(1)">j</a><a href=" data:text/html,x">d</a><iframe src="https://example.org"></iframe>' +
        '<svg><a href="https://example.org">s</a></svg>',
    );
    expect(out).toBe("<p>Text</p><a>j</a><a>d</a>");
  });

  it("packt unbekannte Elemente aus und behält ihren Text", async () => {
    const { sanitizeNotes } = await load();
    expect(sanitizeNotes("<custom-x><font color=red><b>fett</b></font></custom-x>")).toBe("<b>fett</b>");
  });

  it("ist leer ohne Hinweise", async () => {
    const { sanitizeNotes } = await load();
    expect(sanitizeNotes(undefined)).toBe("");
    expect(sanitizeNotes("")).toBe("");
  });
});
