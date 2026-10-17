import { memo, useEffect, useRef } from "react";
import type { Doc, Element, Page as PageT, Rect } from "../core/model";
import type { PageCache } from "../core/render";

export interface PageProps {
  doc: Doc;
  page: PageT;
  cache: PageCache;
  zoom: number;
  visible: boolean;
  hover: Element | null;
  sel: Element | null;
  related: Element[];
  drag: [number, number] | null;
  onPointer: (type: string, page: PageT, x: number, y: number) => void;
}

const corners = [[0, 0], [0.5, 0], [1, 0], [1, 0.5], [1, 1], [0.5, 1], [0, 1], [0, 0.5]];

function PageView({ page, cache, zoom, visible, hover, sel, related, drag, onPointer }: PageProps) {
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    if (!visible || !canvas.current) return;
    const dpr = window.devicePixelRatio || 1;
    const img = cache.get(page, zoom * dpr);
    canvas.current.width = img.width;
    canvas.current.height = img.height;
    canvas.current.getContext("2d")!.drawImage(img, 0, 0);
  }, [visible, zoom, page, cache]);

  const handler = (e: React.MouseEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    onPointer(e.type, page, (e.clientX - r.left) / zoom, (e.clientY - r.top) / zoom);
  };

  const box = (r: Rect, dx = 0, dy = 0): React.CSSProperties => ({
    left: (r[0] + dx) * zoom, top: (r[1] + dy) * zoom, width: r[2] * zoom, height: r[3] * zoom,
  });

  return (
    <div className="page" data-page={page.n} style={{ width: page.w * zoom, height: page.h * zoom }}
      onMouseMove={handler} onMouseDown={handler} onMouseUp={handler}>
      {visible && <canvas ref={canvas} />}
      <div className="overlay">
        {related.map((r, i) => <div key={i} className="box related" style={box(r.r)} />)}
        {hover && <div className="box hover" data-hover={hover.id} style={box(hover.r)} />}
        {sel && (
          <>
            <div className="box sel" data-sel={sel.id} style={box(sel.r)} />
            {corners.map(([fx, fy], i) => (
              <div key={i} className="handle"
                style={{ left: (sel.r[0] + fx * sel.r[2]) * zoom, top: (sel.r[1] + fy * sel.r[3]) * zoom }} />
            ))}
            {drag && <div className="box ghost" data-drag={`${drag[0]},${drag[1]}`} style={box(sel.r, drag[0], drag[1])} />}
          </>
        )}
      </div>
    </div>
  );
}

export const Page = memo(PageView);
