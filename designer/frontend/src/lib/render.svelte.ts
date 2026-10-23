// Laid out pages as a read cache. After every layout run the backend reports a
// manifest with the hashes of the pages, only unknown pages are fetched.

import { EVENT, RenderAPI, on } from "./api";
import type { Manifest, Page } from "./canvas/model";

export type View = "design" | "preview" | "pdf";

export const render = $state({
  view: "design" as View,
  manifest: null as Manifest | null,
  pages: [] as Page[],
  // version increases when images have loaded and a redraw is needed.
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
  if (render.view !== "pdf" && m.view && m.view !== render.view) return; // outdated view
  const seq = ++latest;
  await loadFonts(m);
  const pages = await Promise.all(m.pages.map((r) => page(r.hash)));
  if (seq !== latest) return; // a newer manifest is already on its way
  render.manifest = m;
  render.pages = pages;
}

on<Manifest>(EVENT.render, apply);
RenderAPI.Latest().then((m) => apply(m as unknown as Manifest));
// The backend can still have the view of an earlier session, for example after
// a reload. The UI starts in the draft and reports that.
RenderAPI.SetView(render.view, "");

export function setView(v: View) {
  render.view = v;
  if (v !== "pdf") RenderAPI.SetView(v, "");
}
