import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";

// Ein Build mit beiden Prototypen als eigene Seiten.
export default defineConfig({
  base: "./",
  plugins: [svelte(), react()],
  build: {
    target: "es2022",
    rollupOptions: {
      input: {
        index: resolve(import.meta.dirname, "index.html"),
        svelte: resolve(import.meta.dirname, "svelte.html"),
        react: resolve(import.meta.dirname, "react.html"),
      },
    },
  },
});
