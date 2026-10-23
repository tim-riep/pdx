// Measurement for both prototypes with the same inputs. The measurement talks
// to the UI only through the DOM:
//   [data-viewport]       scroll area
//   [data-page="n"]       page n, content in points times zoom
//   [data-hover]          frame of the element under the mouse (value: ID)
//   [data-sel]            selection (value: ID)
//   [data-drag]           offset while dragging in points, as "dx,dy"
// and through window.__app.setZoom(z).

import { HitIndex } from "./hit";
import type { Doc } from "./model";

declare global {
  interface Window {
    __app?: { setZoom(z: number): void; zoom(): number };
    __bench?: (opts?: { quick?: boolean }) => Promise<BenchResult>;
    __benchResult?: BenchResult;
  }
}

export interface FrameStats {
  frames: number;
  fps: number;
  p50: number;
  p95: number;
  max: number;
  over33: number; // frames over 33 ms
}

export interface LatencyStats {
  n: number;
  p50: number;
  p95: number;
  max: number;
}

export interface BenchResult {
  framework: string;
  userAgent: string;
  dpr: number;
  viewport: [number, number];
  zoom: FrameStats;
  scroll: FrameStats;
  pages: number;
  hover: FrameStats & { latency: LatencyStats; commit: LatencyStats };
  select: LatencyStats & { commit: LatencyStats };
  drag: FrameStats & { latency: LatencyStats; commit: LatencyStats };
  firstPaint: number;
}

const raf = () => new Promise<number>((r) => requestAnimationFrame(r));

function frameStats(ts: number[]): FrameStats {
  const d: number[] = [];
  for (let i = 1; i < ts.length; i++) d.push(ts[i] - ts[i - 1]);
  d.sort((a, b) => a - b);
  const q = (p: number) => d[Math.min(d.length - 1, Math.floor(p * d.length))] ?? 0;
  const dur = ts[ts.length - 1] - ts[0];
  return {
    frames: d.length,
    fps: round((d.length * 1000) / dur),
    p50: round(q(0.5)),
    p95: round(q(0.95)),
    max: round(d[d.length - 1] ?? 0),
    over33: d.filter((x) => x > 33.4).length,
  };
}

function latencyStats(v: number[]): LatencyStats {
  const d = [...v].sort((a, b) => a - b);
  const q = (p: number) => d[Math.min(d.length - 1, Math.floor(p * d.length))] ?? 0;
  return { n: d.length, p50: round(q(0.5)), p95: round(q(0.95)), max: round(d[d.length - 1] ?? 0) };
}

const round = (x: number) => Math.round(x * 10) / 10;

function pageEl(n: number): HTMLElement {
  return document.querySelector(`[data-page="${n}"]`) as HTMLElement;
}

// Point on page n in screen coordinates.
function client(n: number, x: number, y: number): [number, number] {
  const r = pageEl(n).getBoundingClientRect();
  const z = window.__app!.zoom();
  return [r.left + x * z, r.top + y * z];
}

function mouse(type: string, n: number, x: number, y: number): void {
  const [cx, cy] = client(n, x, y);
  const target = document.elementFromPoint(cx, cy) ?? pageEl(n);
  target.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, clientX: cx, clientY: cy, button: 0, buttons: type === "mouseup" ? 0 : 1 }));
}

// Waits for the DOM change that satisfies cond and returns the time since t0.
// This measures the work of event and framework without the frame timing.
function commit(cond: () => boolean, t0: number): Promise<number> {
  if (cond()) return Promise.resolve(performance.now() - t0);
  return new Promise((res) => {
    const mo = new MutationObserver(() => {
      if (cond()) {
        mo.disconnect();
        res(performance.now() - t0);
      }
    });
    mo.observe(document.body, { subtree: true, attributes: true, childList: true });
    setTimeout(() => { mo.disconnect(); res(NaN); }, 2000);
  });
}

// Waits frame by frame until cond is satisfied and returns the time since t0.
async function until(cond: () => boolean, t0: number): Promise<number> {
  for (let i = 0; i < 120; i++) {
    const t = await raf();
    if (cond()) return t - t0;
  }
  return NaN;
}

function attr(sel: string, name: string): string | null {
  return document.querySelector(sel)?.getAttribute(name) ?? null;
}

export function installBench(framework: string, doc: Doc, firstPaint: number): void {
  window.__bench = async (opts = {}) => {
    const n = opts.quick ? 0.25 : 1;
    const app = window.__app!;
    const vp = document.querySelector("[data-viewport]") as HTMLElement;
    app.setZoom(1);
    vp.scrollTop = 0;
    await raf();
    await raf();

    // Zoom: one step per frame between 50 % and 300 %.
    let ts: number[] = [];
    const steps = Math.round(60 * n);
    for (let i = 0; i <= steps; i++) {
      const z = 0.5 + 2.5 * Math.abs(Math.sin((i / steps) * Math.PI));
      app.setZoom(Math.round(z * 100) / 100);
      ts.push(await raf());
    }
    const zoom = frameStats(ts);
    app.setZoom(1);
    await raf();

    // Scrolling over all pages.
    ts = [];
    const max = vp.scrollHeight - vp.clientHeight;
    const frames = Math.round(150 * n);
    for (let i = 0; i <= frames; i++) {
      vp.scrollTop = (max * i) / frames;
      ts.push(await raf());
    }
    const scroll = frameStats(ts);

    // Hover over the elements of page 2, one event per frame.
    const page = doc.pages[1];
    pageEl(page.n).scrollIntoView({ block: "start" });
    await raf();
    await raf();
    const els = page.els.filter((e) => e.r[2] > 4 && e.r[3] > 4);
    const pick = (i: number) => els[(i * 7919) % els.length];
    ts = [];
    const hoverLat: number[] = [], hoverCommit: number[] = [];
    const hovers = Math.round(200 * n);
    for (let i = 0; i < hovers; i++) {
      const e = pick(i);
      const x = e.r[0] + e.r[2] / 2, y = e.r[1] + e.r[3] / 2;
      const t0 = performance.now();
      const want = hitId(page.n, x, y);
      const ok = () => attr("[data-hover]", "data-hover") === want;
      const c = commit(ok, t0);
      mouse("mousemove", page.n, x, y);
      hoverCommit.push(await c);
      const lat = await until(ok, t0);
      hoverLat.push(lat);
      ts.push(performance.now());
    }
    const hover = { ...frameStats(ts), latency: latencyStats(hoverLat), commit: latencyStats(hoverCommit) };

    // Selection by click.
    const selLat: number[] = [], selCommit: number[] = [];
    for (let i = 0; i < Math.round(40 * n); i++) {
      const e = pick(i * 3 + 1);
      const x = e.r[0] + e.r[2] / 2, y = e.r[1] + e.r[3] / 2;
      const want = hitId(page.n, x, y);
      const ok = () => attr("[data-sel]", "data-sel") === want;
      const t0 = performance.now();
      const c = commit(ok, t0);
      mouse("mousedown", page.n, x, y);
      mouse("mouseup", page.n, x, y);
      selCommit.push(await c);
      selLat.push(await until(ok, t0));
    }
    const select = { ...latencyStats(selLat), commit: latencyStats(selCommit) };

    // Dragging: select an element, then one mouse step per frame.
    const e = els[Math.floor(els.length / 2)];
    const sx = e.r[0] + e.r[2] / 2, sy = e.r[1] + e.r[3] / 2;
    mouse("mousedown", page.n, sx, sy);
    mouse("mouseup", page.n, sx, sy);
    await raf();
    mouse("mousedown", page.n, sx, sy);
    ts = [];
    const dragLat: number[] = [], dragCommit: number[] = [];
    const moves = Math.round(120 * n);
    for (let i = 1; i <= moves; i++) {
      const dx = Math.round(Math.sin(i / 10) * 60), dy = Math.round(i / 2);
      const ok = () => attr("[data-drag]", "data-drag") === `${dx},${dy}`;
      const t0 = performance.now();
      const c = commit(ok, t0);
      mouse("mousemove", page.n, sx + dx, sy + dy);
      dragCommit.push(await c);
      dragLat.push(await until(ok, t0));
      ts.push(performance.now());
    }
    mouse("mouseup", page.n, sx, sy + moves / 2);
    const drag = { ...frameStats(ts), latency: latencyStats(dragLat), commit: latencyStats(dragCommit) };

    const res: BenchResult = {
      framework,
      userAgent: navigator.userAgent,
      dpr: window.devicePixelRatio,
      viewport: [vp.clientWidth, vp.clientHeight],
      pages: doc.pages.length,
      zoom, scroll, hover, select, drag, firstPaint: round(firstPaint),
    };
    window.__benchResult = res;
    console.log("bench", JSON.stringify(res));
    return res;
  };

  // With ?bench=1 the measurement runs by itself, the result then appears as
  // text on the page (for browsers without script access, for example Safari).
  if (new URLSearchParams(location.search).has("bench")) {
    setTimeout(async () => {
      const r = await window.__bench!();
      const pre = document.createElement("pre");
      pre.id = "bench-out";
      pre.style.cssText = "position:fixed;inset:40px 20px auto 20px;z-index:9;background:#fff;border:1px solid #888;padding:8px;font:12px ui-monospace,monospace;white-space:pre-wrap";
      const f = (s: FrameStats) => `${s.fps} fps, p95 ${s.p95} ms, max ${s.max} ms, >33ms ${s.over33}`;
      const l = (s: LatencyStats) => `p50 ${s.p50} ms, p95 ${s.p95} ms, max ${s.max} ms`;
      pre.textContent = [
        `${r.framework}, ${r.pages} Seiten, dpr ${r.dpr}, ${r.viewport.join("x")}`,
        `Zoom:    ${f(r.zoom)}`,
        `Scroll:  ${f(r.scroll)}`,
        `Hover:   ${f(r.hover)} | Commit ${l(r.hover.commit)} | Frame ${l(r.hover.latency)}`,
        `Auswahl: Commit ${l(r.select.commit)} | Frame ${l(r.select)}`,
        `Ziehen:  ${f(r.drag)} | Commit ${l(r.drag.commit)} | Frame ${l(r.drag.latency)}`,
        `Start:   ${r.firstPaint} ms`,
        navigator.userAgent,
      ].join("\n");
      document.body.appendChild(pre);
      document.title = "fertig " + r.framework;
    }, 800);
  }

  // Expected ID under a point, computed independently of the UI.
  const indexes = new Map<number, HitIndex>();
  function hitId(n: number, x: number, y: number): string | null {
    let idx = indexes.get(n);
    if (!idx) {
      idx = new HitIndex(doc.pages.find((p) => p.n === n)!);
      indexes.set(n, idx);
    }
    return idx.at(x, y)?.id ?? null;
  }
}
