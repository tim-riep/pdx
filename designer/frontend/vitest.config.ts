import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Komponententests in jsdom. Die Ende-zu-Ende-Tests unter e2e/ laufen mit
// Playwright.
export default defineConfig({
  plugins: [svelte()],
  resolve: process.env.VITEST ? { conditions: ["browser"] } : undefined,
  test: { include: ["src/**/*.test.ts"], environment: "jsdom" },
});
