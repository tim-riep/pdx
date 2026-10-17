// Seitenformat aus internal/pipeline/format.go. Maße in Punkt, Ursprung oben
// links. Jeder Eintrag trägt die ID seines Vorlagenelements.

export type Rect = [number, number, number, number];
export type Border = [number, string, string] | 0;

export interface RectItem { k: "r"; id?: string; r: Rect; bg?: string; b?: Border[] }
export interface TextItem { k: "t"; id?: string; x: number; y: number; f: string; s: number; c: string; t: string[]; a: number[] }
export interface LineItem { k: "l"; id?: string; p: [number, number, number, number]; w: number; c: string; st?: string }
export interface PathItem { k: "p"; id?: string; c: string; r: Rect[] }
export interface ImageItem { k: "i"; id?: string; r: Rect; i: string }
export type DrawItem = RectItem | TextItem | LineItem | PathItem | ImageItem;

export interface Area { id: string; r: Rect; k: "block" | "inline" | "text" | "image" | "code" }

export interface Page { n: number; w: number; h: number; draw: DrawItem[]; els: Area[] }

export interface PageRef { n: number; w: number; h: number; hash: string }
export interface FontRef { key: string; family: string }
export interface Problem { level: string; id: string; msg: string }

export interface Manifest {
  view: string;
  rev: number;
  sample: string;
  pages: PageRef[];
  fonts: FontRef[];
  problems: Problem[];
  millis: number;
  compiled: boolean;
}
