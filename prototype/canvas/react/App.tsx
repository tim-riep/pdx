import { useCallback, useMemo, useRef, useState } from "react";
import type { Doc, Element, Page as PageT } from "../core/model";
import { PageCache } from "../core/render";
import { HitIndex } from "../core/hit";
import { Page } from "./Page";

type Ref = { n: number; el: Element } | null;
type Drag = { n: number; x: number; y: number; dx: number; dy: number; active: boolean } | null;

const none: Element[] = [];

export function App({ doc }: { doc: Doc }) {
  const cache = useMemo(() => new PageCache(doc), [doc]);
  const index = useMemo(() => new Map(doc.pages.map((p) => [p.n, new HitIndex(p)])), [doc]);

  const [zoom, setZoom] = useState(1);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(800);
  const [hover, setHover] = useState<Ref>(null);
  const [sel, setSel] = useState<Ref>(null);
  const [drag, setDrag] = useState<Drag>(null);
  const [last, setLast] = useState("");

  // Aktuelle Werte für Ereignisse, ohne die Rückruffunktion neu zu bauen.
  const state = useRef({ zoom, hover, sel, drag });
  state.current = { zoom, hover, sel, drag };
  window.__app = { setZoom, zoom: () => state.current.zoom };

  const viewport = useCallback((el: HTMLDivElement | null) => {
    if (!el) return;
    setHeight(el.clientHeight);
    new ResizeObserver(() => setHeight(el.clientHeight)).observe(el);
  }, []);

  const tops = useMemo(() => {
    let y = 24;
    return doc.pages.map((p) => {
      const t = y;
      y += p.h * zoom + 24;
      return t;
    });
  }, [doc, zoom]);

  const onPointer = useCallback((type: string, page: PageT, x: number, y: number) => {
    const { hover, sel, drag } = state.current;
    const hit = index.get(page.n)!.at(x, y) ?? null;
    if (type === "mousemove") {
      if (drag) {
        setDrag({ ...drag, dx: Math.round(x - drag.x), dy: Math.round(y - drag.y), active: true });
        return;
      }
      if (hit !== (hover?.el ?? null)) setHover(hit ? { n: page.n, el: hit } : null);
    } else if (type === "mousedown") {
      if (sel && hit && sel.n === page.n && hit === sel.el) {
        setDrag({ n: page.n, x, y, dx: 0, dy: 0, active: false });
      } else {
        setSel(hit ? { n: page.n, el: hit } : null);
      }
    } else if (type === "mouseup" && drag) {
      if (drag.dx || drag.dy) setLast(`${sel?.el.id} verschieben um ${drag.dx}, ${drag.dy} pt`);
      setDrag(null);
    }
  }, [index]);

  const related = useMemo(() => {
    const out = new Map<number, Element[]>();
    if (!sel) return out;
    for (const p of doc.pages) out.set(p.n, p.els.filter((e) => e.id === sel.el.id && e !== sel.el));
    return out;
  }, [doc, sel]);

  return (
    <div className="shell">
      <div className="toolbar">
        <strong>React</strong>
        <button onClick={() => setZoom((z) => Math.max(0.25, z / 1.25))}>−</button>
        <span>{Math.round(zoom * 100)} %</span>
        <button onClick={() => setZoom((z) => Math.min(4, z * 1.25))}>+</button>
        <span className="info">{sel ? `Auswahl: ${sel.el.id}` : "keine Auswahl"} {last}</span>
      </div>
      <div className="viewport" data-viewport ref={viewport}
        onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}>
        <div className="pages">
          {doc.pages.map((p, i) => {
            const t = tops[i], b = t + p.h * zoom;
            const visible = b > scrollTop - height && t < scrollTop + 2 * height;
            const dragged = drag?.active && drag.n === p.n ? drag : null;
            return (
              <Page key={p.n} doc={doc} page={p} cache={cache} zoom={zoom} visible={visible}
                hover={hover?.n === p.n ? hover.el : null}
                sel={sel?.n === p.n ? sel.el : null}
                related={related.get(p.n) ?? none}
                drag={dragged ? [dragged.dx, dragged.dy] : null}
                onPointer={onPointer} />
            );
          })}
        </div>
      </div>
    </div>
  );
}
