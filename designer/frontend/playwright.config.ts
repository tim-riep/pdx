import { defineConfig, devices } from "@playwright/test";

// Ende-zu-Ende gegen den Server-Build des Designers (Wails mit -tags server):
// dieselben Services, Bindungen über HTTP, Ereignisse über WebSocket.
const port = Number(process.env.PDX_E2E_PORT ?? 8799);
const bin = `../out/designer-server${process.platform === "win32" ? ".exe" : ""}`;

export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  reporter: [["list"]],
  use: { baseURL: `http://localhost:${port}`, trace: "retain-on-failure" },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } } },
    { name: "webkit", use: { ...devices["Desktop Safari"], viewport: { width: 1440, height: 900 } } },
  ],
  webServer: {
    command: `cd .. && CGO_ENABLED=0 go build -tags server -o ${bin} . && WAILS_SERVER_PORT=${port} ${bin}`,
    url: `http://localhost:${port}/health`,
    timeout: 180_000,
    reuseExistingServer: false,
  },
});
