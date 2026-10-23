import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Component tests in jsdom. The end-to-end tests under e2e/ run with
// Playwright.
export default defineConfig({
  plugins: [svelte()],
  resolve: process.env.VITEST ? { conditions: ["browser"] } : undefined,
  test: { include: ["src/**/*.test.ts"], environment: "jsdom" },
});
