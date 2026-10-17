// Gesetzte Seiten als Lesecache. Das Backend meldet nach jedem Setzen ein
// Manifest mit den Hashes der Seiten, nur unbekannte Seiten werden geholt.

import { EVENT, RenderAPI, on } from "./api";
import type { Manifest, Page } from "./canvas/model";

export type View = "design" | "preview" | "pdf";

export const render = $state({
  view: "design" as View,
  manifest: null as Manifest | null,
  pages: [] as Page[],
  // version steigt, wenn Bilder nachgeladen sind und neu gezeichnet werden muss.
  version: 0,
});

const pageCache = new Map<string, Page>();
const fontsLoaded = new Set<string>();
const images = new Map<string, HTMLImageElement>();

export function image(key: string): HTMLImageElement | undefined {
  let img = images.get(key);
  if (!img) {
    img = new Image();
    img.onload = () => render.version++;
    img.src = `/pdx/image/${key}`;
    images.set(key, img);
  }
  return img;
}

async function loadFonts(m: Manifest) {
  await Promise.all(
    m.fonts
      .filter((f) => !fontsLoaded.has(f.key))
      .map(async (f) => {
        const face = new FontFace(f.family, `url(/pdx/font/${f.key})`);
        await face.load();
        document.fonts.add(face);
        fontsLoaded.add(f.key);
      }),
  );
}

async function page(hash: string): Promise<Page> {
  const hit = pageCache.get(hash);
  if (hit) return hit;
  const p: Page = await (await fetch(`/pdx/page/${hash}`)).json();
  pageCache.set(hash, p);
  return p;
}

let latest = -1;

async function apply(m: Manifest) {
  if (!m || !m.pages) return;
  if (render.view !== "pdf" && m.view && m.view !== render.view) return; // veraltete Ansicht
  const seq = ++latest;
  await loadFonts(m);
  const pages = await Promise.all(m.pages.map((r) => page(r.hash)));
  if (seq !== latest) return; // ein neueres Manifest ist schon unterwegs
  render.manifest = m;
  render.pages = pages;
}

on<Manifest>(EVENT.render, apply);
RenderAPI.Latest().then((m) => apply(m as unknown as Manifest));

export function setView(v: View) {
  render.view = v;
  if (v !== "pdf") RenderAPI.SetView(v, "");
}
