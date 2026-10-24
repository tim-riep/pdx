import { defineConfig, devices } from "@playwright/test";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

// End-to-end against the server build of the designer (Wails with -tags server):
// the same services, bindings over HTTP, events over WebSocket.
const port = Number(process.env.PDX_E2E_PORT ?? 8799);
// Own data directory, so tests do not touch the user profile.
const data = process.env.PDX_DESIGNER_DATA ?? mkdtempSync(join(tmpdir(), "pdx-e2e-"));
process.env.PDX_E2E_DATA = data;
// Absolute paths and environment via env instead of shell syntax: Playwright
// starts the commands on Windows with cmd.exe.
const exe = process.platform === "win32" ? ".exe" : "";
const designerDir = resolve(import.meta.dirname, "..");
const rootDir = resolve(designerDir, "..");
const bin = resolve(rootDir, "out", `designer-server${exe}`);
// A stand-in with a fixed report replaces veraPDF, so the tests run fast
// and without Docker, on Windows via a .cmd file.
// With PDX_E2E_REAL_VERAPDF=1 the real veraPDF checks (local or Docker).
const stub = process.env.PDX_E2E_REAL_VERAPDF ? "" : join(import.meta.dirname, "e2e", "fixtures", `verapdf-attrappe${process.platform === "win32" ? ".cmd" : ""}`);
const verapdf = process.env.PDX_VERAPDF ?? stub;
process.env.PDX_E2E_VERAPDF = verapdf;

// A real pdx-server for publishing, with a test key and
// empty storage in the data directory of the tests.
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
const serverBin = resolve(rootDir, "out", `pdx-server-e2e${exe}`);

// A stand-in for GitHub answers the update check, so the tests make no
// requests to the internet.
const releasePort = Number(process.env.PDX_E2E_RELEASE_PORT ?? 8797);
process.env.PDX_E2E_RELEASE = `http://127.0.0.1:${releasePort}`;

export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  workers: 1, // one backend with one document for all tests
  reporter: [["list"]],
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "webkit", use: { ...devices["Desktop Safari"], viewport: { width: 1440, height: 900 } } },
  ],
  webServer: [
    {
      command: `go build -tags server -o "${bin}" . && "${bin}"`,
      cwd: designerDir,
      env: {
        ...process.env,
        CGO_ENABLED: "0",
        PDX_DESIGNER_DATA: data,
        PDX_VERAPDF: verapdf,
        PDX_UPDATE_URL: `http://127.0.0.1:${releasePort}/release`,
        WAILS_SERVER_PORT: String(port),
      } as Record<string, string>,
      url: `http://localhost:${port}/health`,
      timeout: 180_000,
      reuseExistingServer: false,
    },
    {
      command: `go build -o "${serverBin}" ./cmd/pdx-server && "${serverBin}" -config "${serverConfig}"`,
      cwd: rootDir,
      env: { ...process.env, CGO_ENABLED: "0" } as Record<string, string>,
      url: `http://127.0.0.1:${serverPort}/healthz`,
      timeout: 120_000,
      reuseExistingServer: false,
    },
    {
      command: `node "${join(import.meta.dirname, "e2e", "fixtures", "release-server.mjs")}" ${releasePort}`,
      url: `http://127.0.0.1:${releasePort}/health`,
      timeout: 30_000,
      reuseExistingServer: false,
    },
  ],
});
