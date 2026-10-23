// Data format of the prototype, written by prototype/canvas/export.
// All sizes in points, origin top left.

export type Rect = [number, number, number, number];

export interface RectItem {
  k: "r";
  id?: string;
  r: Rect;
  bg?: string;
  // Borders top, right, bottom, left as [width, color] or 0.
  b?: ([number, string] | 0)[];
}

export interface TextItem {
  k: "t";
  id?: string;
  x: number;
  y: number;
  f: number;
  s: number;
  c: string;
  t: string[];
  a: number[];
}

export interface LineItem {
  k: "l";
  id?: string;
  p: [number, number, number, number];
  w: number;
  c: string;
}

export interface PathItem {
  k: "p";
  id?: string;
  c: string;
  r: Rect[];
}

export type DrawItem = RectItem | TextItem | LineItem | PathItem;

export interface Element {
  id: string;
  r: Rect;
}

export interface Page {
  n: number;
  w: number;
  h: number;
  draw: DrawItem[];
  els: Element[];
}

export interface Doc {
  fonts: { file: string; family: string }[];
  pages: Page[];
}

// loadDoc loads the area tree and registers the fonts as FontFace.
export async function loadDoc(url = "data/musterrechnung.json"): Promise<Doc> {
  const doc: Doc = await (await fetch(url)).json();
  await Promise.all(
    doc.fonts.map(async (f) => {
      const face = new FontFace(f.family, `url(fonts/${f.file})`);
      await face.load();
      document.fonts.add(face);
    }),
  );
  // Load test: ?x=10 multiplies the pages.
  const x = Number(new URLSearchParams(location.search).get("x") ?? "1");
  if (x > 1) {
    const base = doc.pages;
    doc.pages = [];
    for (let i = 0; i < x; i++) {
      for (const p of base) doc.pages.push({ ...p, n: doc.pages.length + 1, els: p.els.map((e) => ({ ...e })) });
    }
  }
  return doc;
}
