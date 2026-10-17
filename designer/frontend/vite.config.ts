import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  plugins: [svelte(), wails("./bindings")],
  server: { port: Number(process.env.WAILS_VITE_PORT ?? 9245), strictPort: true },
  build: { target: "es2022", chunkSizeWarningLimit: 2048 },
});
