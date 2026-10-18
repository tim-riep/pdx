import { defineConfig, devices } from "@playwright/test";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Ende-zu-Ende gegen den Server-Build des Designers (Wails mit -tags server):
// dieselben Services, Bindungen über HTTP, Ereignisse über WebSocket.
const port = Number(process.env.PDX_E2E_PORT ?? 8799);
// Eigenes Datenverzeichnis, damit Tests das Benutzerprofil nicht berühren.
const data = process.env.PDX_DESIGNER_DATA ?? mkdtempSync(join(tmpdir(), "pdx-e2e-"));
process.env.PDX_E2E_DATA = data;
const bin = `../out/designer-server${process.platform === "win32" ? ".exe" : ""}`;
// veraPDF ersetzt eine Attrappe mit festem Bericht, damit die Tests schnell
// und ohne Docker laufen. Unter Windows fehlt die Shell, dort entfällt der Test.
// Mit PDX_E2E_REAL_VERAPDF=1 prüft das echte veraPDF (lokal oder Docker).
const stub = process.env.PDX_E2E_REAL_VERAPDF || process.platform === "win32" ? "" : join(import.meta.dirname, "e2e/fixtures/verapdf-attrappe");
const verapdf = process.env.PDX_VERAPDF ?? stub;

// Ein echter pdx-server für das Veröffentlichen, mit Testschlüssel und
// leerer Ablage im Datenverzeichnis der Tests.
const serverPort = Number(process.env.PDX_E2E_SERVER_PORT ?? 8798);
export const e2eServerKey = "e2e-testschluessel-nur-fuer-playwright";
process.env.PDX_E2E_SERVER = `http://127.0.0.1:${serverPort}`;
mkdirSync(join(data, "pdx-server", "vorlagen"), { recursive: true });
const serverConfig = join(data, "pdx-server", "konfiguration.json");
writeFileSync(serverConfig, JSON.stringify({
  addr: `127.0.0.1:${serverPort}`,
  templateDir: join(data, "pdx-server", "vorlagen"),
  keys: [{ name: "e2e", sha256: createHash("sha256").update(e2eServerKey).digest("hex"), rights: ["templates"] }],
}));
const serverBin = `../out/pdx-server-e2e${process.platform === "win32" ? ".exe" : ""}`;

export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  workers: 1, // ein Backend mit einem Dokument für alle Tests
  reporter: [["list"]],
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "webkit", use: { ...devices["Desktop Safari"], viewport: { width: 1440, height: 900 } } },
  ],
  webServer: [
    {
      command: `cd .. && CGO_ENABLED=0 go build -tags server -o ${bin} . && PDX_DESIGNER_DATA=${data} PDX_VERAPDF=${verapdf} WAILS_SERVER_PORT=${port} ${bin}`,
      url: `http://localhost:${port}/health`,
      timeout: 180_000,
      reuseExistingServer: false,
    },
    {
      command: `cd ../.. && CGO_ENABLED=0 go build -o out/pdx-server-e2e ./cmd/pdx-server && cd designer && ${serverBin} -config ${serverConfig}`,
      url: `http://127.0.0.1:${serverPort}/healthz`,
      timeout: 120_000,
      reuseExistingServer: false,
    },
  ],
});
