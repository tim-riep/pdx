import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// bundled writes the packages from node_modules that end up in the bundle to
// bundled-packages.json. The third-party notices are built from it.
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
      // The pdf.js worker is added as a file.
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
