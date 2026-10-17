// Führt alle Messungen nacheinander in einem iframe aus und zeigt eine
// Übersicht. Für Browser, in denen nur abgelesen werden kann.

import type { BenchResult } from "./bench";

const runs = [
  ["svelte", 1], ["react", 1], ["svelte", 10], ["react", 10],
] as const;

const out = document.getElementById("out")!;
const frame = document.getElementById("frame") as HTMLIFrameElement;
const rows: string[] = [];

function row(r: BenchResult): string {
  return [
    r.framework, r.pages,
    `${r.zoom.fps}/${r.zoom.p95}`, `${r.scroll.fps}/${r.scroll.p95}`,
    `${r.hover.fps}/${r.hover.commit.p95}`, `${r.select.commit.p50}/${r.select.commit.p95}`,
    `${r.drag.fps}/${r.drag.commit.p95}/${r.drag.max}`, r.firstPaint,
  ].join(" | ");
}

async function main() {
  out.textContent = "läuft ...";
  for (const [fw, x] of runs) {
    frame.src = `${fw}.html?x=${x}`;
    await new Promise((r) => (frame.onload = r));
    const w = frame.contentWindow as Window;
    while (!w.__bench) await new Promise((r) => setTimeout(r, 100));
    await new Promise((r) => setTimeout(r, 500));
    rows.push(row(await w.__bench()));
    out.textContent = rows.join("\n") + "\nläuft ...";
  }
  out.textContent = [
    "Framework | Seiten | Zoom fps/p95 | Scroll fps/p95 | Hover fps/Commit p95 | Auswahl Commit p50/p95 | Ziehen fps/Commit p95/max Frame | Start ms",
    ...rows,
    navigator.userAgent,
  ].join("\n");
  document.title = "Messung fertig";
}

main();
