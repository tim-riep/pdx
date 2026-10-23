// Rendert die App-Icons und das Social-Media-Bild aus icon.svg und
// banner.html mit dem Chromium von Playwright (aus designer/frontend).
// Aufruf im Ordner designer: node packaging/icon/render.mjs
// Danach baut iconutil das .icns (macOS) und mkico.go das .ico.
import { chromium } from "../../frontend/node_modules/playwright/index.mjs";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const svg = readFileSync(join(here, "icon.svg"), "utf8");
const sizes = [16, 24, 32, 48, 64, 128, 256, 512, 1024];
mkdirSync(join(here, "png"), { recursive: true });

const browser = await chromium.launch();
const page = await browser.newPage({ deviceScaleFactor: 1 });
for (const size of sizes) {
  // Bis 32 Pixel die vereinfachte Fassung. Bis 48 Pixel ohne den Rand des
  // macOS-Rasters, damit das Motiv die Fläche nutzt.
  const small = size <= 32 ? ' class="small"' : "";
  const view = size <= 48 ? 'viewBox="80 80 864 880"' : 'viewBox="0 0 1024 1024"';
  const html = `<!doctype html><html><body style="margin:0;background:transparent">${svg
    .replace('<svg ', `<svg${small} `)
    .replace('viewBox="0 0 1024 1024"', view)
    .replace('width="1024" height="1024"', `width="${size}" height="${size}"`)}</body></html>`;
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(html);
  await page.locator("svg").screenshot({ path: join(here, "png", `${size}.png`), omitBackground: true });
}

// macOS: Iconset im Raster mit Rand für alle Größen, daraus das .icns.
const iconset = join(here, "pdx-designer.iconset");
rmSync(iconset, { recursive: true, force: true });
mkdirSync(iconset);
for (const [base, scale] of [16, 32, 128, 256, 512].flatMap((b) => [[b, 1], [b, 2]])) {
  const size = base * scale;
  const small = size <= 32 ? ' class="small"' : "";
  const html = `<!doctype html><html><body style="margin:0;background:transparent">${svg
    .replace('<svg ', `<svg${small} `)
    .replace('width="1024" height="1024"', `width="${size}" height="${size}"`)}</body></html>`;
  await page.setViewportSize({ width: size, height: size });
  await page.setContent(html);
  const name = `icon_${base}x${base}${scale === 2 ? "@2x" : ""}.png`;
  await page.locator("svg").screenshot({ path: join(iconset, name), omitBackground: true });
}
if (process.platform === "darwin") {
  execFileSync("iconutil", ["-c", "icns", iconset, "-o", join(here, "pdx-designer.icns")]);
}
rmSync(iconset, { recursive: true, force: true });

// Windows: .ico aus den PNG-Dateien.
execFileSync("go", ["run", join(here, "mkico.go"), join(here, "pdx-designer.ico"),
  ...[16, 24, 32, 48, 64, 128, 256].map((n) => join(here, "png", `${n}.png`))], { stdio: "inherit" });

// Social-Media-Bild für GitHub (1280 x 640).
await page.setViewportSize({ width: 1280, height: 640 });
await page.goto(pathToFileURL(join(here, "banner.html")).href);
await page.evaluate(() => document.fonts.ready);
mkdirSync(join(here, "..", "..", "..", "docs", "assets"), { recursive: true });
await page.screenshot({ path: join(here, "..", "..", "..", "docs", "assets", "social-preview.png") });
await browser.close();
console.log("png/", sizes.join(", "), "und docs/assets/social-preview.png");
