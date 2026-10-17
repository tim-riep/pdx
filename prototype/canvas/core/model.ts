// Datenformat des Prototyps, geschrieben von prototype/canvas/export.
// Alle Maße in Punkt, Ursprung oben links.

export type Rect = [number, number, number, number];

export interface RectItem {
  k: "r";
  id?: string;
  r: Rect;
  bg?: string;
  // Rahmen oben, rechts, unten, links als [Breite, Farbe] oder 0.
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

// loadDoc lädt den Area Tree und registriert die Schriften als FontFace.
export async function loadDoc(url = "data/musterrechnung.json"): Promise<Doc> {
  const doc: Doc = await (await fetch(url)).json();
  await Promise.all(
    doc.fonts.map(async (f) => {
      const face = new FontFace(f.family, `url(fonts/${f.file})`);
      await face.load();
      document.fonts.add(face);
    }),
  );
  // Lasttest: ?x=10 vervielfacht die Seiten.
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
