import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// bundled schreibt die Pakete aus node_modules, die im Bündel landen, nach
// bundled-packages.json. Daraus entstehen die Drittanbieterhinweise.
function bundled(): Plugin {
  return {
    name: "pdx-bundled-packages",
    apply: "build",
    generateBundle(_, bundle) {
      const names = new Set<string>();
      for (const out of Object.values(bundle)) {
        const ids = out.type === "chunk" ? Object.keys(out.modules) : [];
        for (const id of ids) {
          const m = /node_modules\/((?:@[^/]+\/)?[^/]+)\//.exec(id.replace(/\\/g, "/"));
          if (m) names.add(m[1]);
        }
      }
      // Der Worker von pdf.js kommt als Datei dazu.
      if (Object.keys(bundle).some((f) => f.includes("pdf.worker"))) names.add("pdfjs-dist");
      const list = [...names].sort().map((name) => {
        const pkg = JSON.parse(readFileSync(join(import.meta.dirname, "node_modules", name, "package.json"), "utf8"));
        return { name, version: pkg.version, license: pkg.license ?? "" };
      });
      writeFileSync(join(import.meta.dirname, "bundled-packages.json"), JSON.stringify(list, null, 2) + "\n");
    },
  };
}

export default defineConfig({
  plugins: [svelte(), wails("./bindings"), bundled()],
  server: { port: Number(process.env.WAILS_VITE_PORT ?? 9245), strictPort: true },
  build: { target: "es2022", chunkSizeWarningLimit: 2048 },
});
