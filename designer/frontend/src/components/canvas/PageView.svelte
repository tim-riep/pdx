<script lang="ts">
  import type { Area, Page, Rect } from "../../lib/canvas/model";
  import { drawPage, type ImageSource } from "../../lib/canvas/render";
  import { HitIndex } from "../../lib/canvas/hit";
  import { select, selection } from "../../lib/selection.svelte";
  import { DATA_MIME } from "../../lib/data.svelte";
  import { ELEMENT_MIME, elementName, insertElement, type Kind } from "../../lib/insert";
  import { dropTarget, type Target } from "../../lib/drop";
  import { PT_PER_MM, fmtMM, gaps, snap, toMM, type Guide } from "../../lib/geometry";
  import { apply, node, tpl, typeOf } from "../../lib/template.svelte";
  import { render } from "../../lib/render.svelte";
  import { TemplateAPI, message, type ViewNode } from "../../lib/api";
  import { ui } from "../../lib/project.svelte";
  import { t } from "../../lib/i18n";

  let { page, hash, zoom, grid, visible, bitmap, image }: {
    page: Page; hash: string; zoom: number; grid: boolean; visible: boolean;
    bitmap: (p: Page, key: string, scale: number, draw: (c: HTMLCanvasElement) => void) => HTMLCanvasElement;
    image: ImageSource;
  } = $props();

  let canvas: HTMLCanvasElement | undefined = $state();
  let pageEl: HTMLElement | undefined = $state();
  const index = $derived(new HitIndex(page));

  $effect(() => {
    if (!visible || !canvas) return;
    const scale = zoom * (window.devicePixelRatio || 1);
    const img = bitmap(page, hash, scale, (c) => {
      c.width = Math.ceil(page.w * scale);
      c.height = Math.ceil(page.h * scale);
      drawPage(c.getContext("2d")!, page, scale, image);
    });
    canvas.width = img.width;
    canvas.height = img.height;
    canvas.getContext("2d")!.drawImage(img, 0, 0);
  });

  const design = $derived(render.view === "design");

  // Bereiche des ausgewählten und des überfahrenen Elements auf dieser Seite.
  const selected = $derived(selection.ref ? page.els.filter((a) => a.id === selection.ref) : []);
  const hovered = $derived(
    selection.hover && selection.hover !== selection.ref ? page.els.filter((a) => a.id === selection.hover) : [],
  );
  const current = $derived(
    selected.length ? (selected[selection.instance?.page === page.n ? selection.instance.k : 0] ?? selected[0]).r : null,
  );

  // Rahmen der Teilformulare im Entwurf, fließende mit Pfeil für die Richtung.
  const frames = $derived.by(() => {
    if (!design || !tpl.snapshot) return [];
    const seen = new Set<string>();
    const out: { a: Area; flowed: boolean }[] = [];
    for (const a of page.els) {
      const n = node(a.id);
      if (n?.name !== "subform" || a.k !== "block") continue;
      const key = `${a.id}@${a.r.join(",")}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({ a, flowed: n.attrs.layout === "flowed" });
    }
    return out;
  });

  function pagePoint(e: MouseEvent): [number, number] {
    const r = pageEl!.getBoundingClientRect();
    return [(e.clientX - r.left) / zoom, (e.clientY - r.top) / zoom];
  }

  // Positioniert ist ein Element, dessen Eltern ihre Kinder positionieren.
  function isPositioned(n: ViewNode | undefined): boolean {
    const p = node(n?.parent);
    if (!p) return false;
    return p.type === "pageMaster/header" || p.type === "pageMaster/footer" || (p.name === "subform" && p.attrs.layout === "positioned");
  }
  const canMove = (n: ViewNode | undefined) =>
    !!n && !!n.parent && n.name !== "body" && !n.type.startsWith("pageMaster") && node(n.parent)?.name !== "template";

  // Ziehen: Verschieben oder Größe ändern positionierter Elemente,
  // Umsortieren fließender.
  type Drag = {
    mode: "move" | "resize" | "reorder";
    handle: [number, number]; // Kante beim Größe ändern: 0 links oder oben, 1 rechts oder unten
    ref: string;
    start: [number, number];
    rect: Rect; // Bereich zu Beginn
    orig: { x: number; y: number; w: number; h: number }; // Vorlagenwerte in mm
    live: Rect;
    active: boolean;
    guides: Guide[];
    target: Target | null;
  };
  let drag = $state<Drag | null>(null);

  function siblings(ref: string): Rect[] {
    const n = node(ref);
    const p = node(n?.parent);
    if (!p) return [];
    const out: Rect[] = [];
    for (const c of p.children) {
      if (!c?.ref || c.ref === ref) continue;
      for (const a of page.els) if (a.id === c.ref) out.push(a.r);
    }
    const parentArea = page.els.find((a) => a.id === p.ref);
    if (parentArea) out.push(parentArea.r);
    return out;
  }

  function startDrag(e: MouseEvent, mode: Drag["mode"], handle: [number, number]) {
    const n = node(selection.ref);
    if (!n || !current) return;
    const orig = {
      x: toMM(n.attrs.x) ?? 0,
      y: toMM(n.attrs.y) ?? 0,
      w: toMM(n.attrs.w) ?? current[2] / PT_PER_MM,
      h: toMM(n.attrs.h) ?? current[3] / PT_PER_MM,
    };
    drag = { mode, handle, ref: n.ref, start: pagePoint(e), rect: current, orig, live: current, active: false, guides: [], target: null };
    e.stopPropagation();
    e.preventDefault();
    window.addEventListener("mousemove", dragMove);
    window.addEventListener("mouseup", dragEnd, { once: true });
  }

  // gridDelta rundet eine Verschiebung auf ganze Millimeter.
  const gridDelta = (d: number) => Math.round(d / PT_PER_MM) * PT_PER_MM;

  function dragMove(e: MouseEvent) {
    if (!drag) return;
    const [px, py] = pagePoint(e);
    const dx = px - drag.start[0], dy = py - drag.start[1];
    if (!drag.active && Math.hypot(dx, dy) * zoom < 3) return;
    drag.active = true;
    if (drag.mode === "reorder") {
      drag.live = [drag.rect[0] + dx, drag.rect[1] + dy, drag.rect[2], drag.rect[3]];
      const tgt = dropTarget(model, page, index, px, py, node(drag.ref)?.name ?? "field");
      drag.target = tgt && !tgt.positioned && !isInside(tgt.parent, drag.ref) ? tgt : null;
      return;
    }
    const [x, y, w, h] = drag.rect;
    let r: Rect;
    if (drag.mode === "move") r = [x + dx, y + dy, w, h];
    else {
      const [hx, hy] = drag.handle;
      r = [
        hx === 0 ? x + dx : x,
        hy === 0 ? y + dy : y,
        hx === 0 ? w - dx : hx === 1 ? w + dx : w,
        hy === 0 ? h - dy : hy === 1 ? h + dy : h,
      ];
    }
    // Einrasten an Nachbarn und Elternelement, sonst am Raster.
    const s = snap(r, siblings(drag.ref), 5 / zoom);
    if (drag.mode === "move") {
      r = [r[0] + s.dx, r[1] + s.dy, r[2], r[3]];
      if (grid && !s.dx) r[0] = x + gridDelta(r[0] - x);
      if (grid && !s.dy) r[1] = y + gridDelta(r[1] - y);
    } else {
      const [hx, hy] = drag.handle;
      if (hx === 1) r[2] += s.dx;
      if (hx === 0) { r[0] += s.dx; r[2] -= s.dx; }
      if (hy === 1) r[3] += s.dy;
      if (hy === 0) { r[1] += s.dy; r[3] -= s.dy; }
      r[2] = Math.max(r[2], 2 * PT_PER_MM);
      r[3] = Math.max(r[3], 2 * PT_PER_MM);
    }
    drag.guides = s.guides;
    drag.live = r;
  }

  function isInside(ref: string, ancestor: string): boolean {
    for (let n = node(ref); n; n = node(n.parent)) if (n.ref === ancestor) return true;
    return false;
  }

  async function dragEnd() {
    window.removeEventListener("mousemove", dragMove);
    const d = drag;
    drag = null;
    if (!d?.active) return;
    if (d.mode === "reorder") {
      if (!d.target) return;
      const parent = node(d.target.parent)!;
      const me = node(d.ref)!;
      let i = d.target.index;
      const old = parent.children.findIndex((c) => c?.ref === d.ref);
      if (me.parent === parent.ref && i > old) i--;
      await apply(t("cmd.reorder"), { op: "move", ref: d.ref, parent: parent.ref, index: i });
      return;
    }
    const [x, y, w, h] = d.live, [ox, oy, ow, oh] = d.rect;
    const mm = (v: number) => v / PT_PER_MM;
    const cmds: { op: string; ref: string; name: string; value: string }[] = [];
    const set = (name: string, v: number) => cmds.push({ op: "setAttr", ref: d.ref, name, value: fmtMM(v) });
    if (Math.abs(x - ox) > 0.01) set("x", d.orig.x + mm(x - ox));
    if (Math.abs(y - oy) > 0.01) set("y", d.orig.y + mm(y - oy));
    if (d.mode === "resize") {
      if (Math.abs(w - ow) > 0.01) set("w", d.orig.w + mm(w - ow));
      if (Math.abs(h - oh) > 0.01) set("h", d.orig.h + mm(h - oh));
    }
    if (cmds.length) await apply(t(d.mode === "move" ? "cmd.move" : "cmd.resize"), { op: "batch", cmds: cmds as never });
  }

  function move(e: MouseEvent) {
    if (drag) return;
    const hit = index.at(...pagePoint(e));
    const id = hit?.id ?? "";
    if (id !== selection.hover) selection.hover = id;
  }

  function down(e: MouseEvent) {
    if (e.button !== 0) return;
    const p = pagePoint(e);
    const hit: Area | undefined = index.at(...p);
    // Ein Klick auf die Auswahl beginnt das Ziehen.
    if (design && current && hit?.id === selection.ref && canMove(node(selection.ref))) {
      startDrag(e, isPositioned(node(selection.ref)) ? "move" : "reorder", [0.5, 0.5]);
      return;
    }
    if (hit) select(hit.id, { page: page.n, k: page.els.filter((a) => a.id === hit.id).indexOf(hit) });
    else select("");
  }

  // Ablage gezogener Datenknoten und Bausteine.
  let target = $state<Target | null>(null);
  const model = {
    node,
    type: (n: ViewNode) => typeOf(n),
    body: () => Object.values(tpl.snapshot?.nodes ?? {}).find((n) => n?.name === "body"),
  };

  function dragover(e: DragEvent) {
    const types = e.dataTransfer?.types ?? [];
    if (!types.includes(DATA_MIME) && !types.includes(ELEMENT_MIME)) return;
    e.preventDefault();
    e.dataTransfer!.dropEffect = "copy";
    target = dropTarget(model, page, index, ...pagePoint(e));
  }

  async function drop(e: DragEvent) {
    const path = e.dataTransfer?.getData(DATA_MIME);
    const kind = e.dataTransfer?.getData(ELEMENT_MIME) as Kind | "";
    const tg = kind
      ? dropTarget(model, page, index, ...pagePoint(e), elementName[kind])
      : (target ?? dropTarget(model, page, index, ...pagePoint(e)));
    target = null;
    if (!tg || (!path && !kind)) return;
    e.preventDefault();
    if (kind) {
      await insertElement(kind, tg.parent, tg.index, tg.x, tg.y);
      return;
    }
    try {
      const res = await TemplateAPI.Drop({ path: path!, parent: tg.parent, index: tg.index, x: tg.x, y: tg.y });
      tpl.history = res.history;
      if (res.select) select(res.select);
      ui.error = "";
    } catch (err) {
      ui.error = message(err);
    }
  }

  const box = (r: number[]) =>
    `left:${r[0] * zoom}px;top:${r[1] * zoom}px;width:${r[2] * zoom}px;height:${r[3] * zoom}px`;

  const handles: [number, number][] = [[0, 0], [0.5, 0], [1, 0], [1, 0.5], [1, 1], [0.5, 1], [0, 1], [0, 0.5]];
  const cursor = (hx: number, hy: number) => (hx === 0.5 ? "ns" : hy === 0.5 ? "ew" : hx === hy ? "nwse" : "nesw") + "-resize";
  const resizable = $derived(design && !!current && isPositioned(node(selection.ref)));
  const spacing = $derived(drag?.active && drag.mode !== "reorder" ? gaps(drag.live, siblings(drag.ref)) : []);
</script>

<div class="page" data-page={page.n} style="width:{page.w * zoom}px;height:{page.h * zoom}px" bind:this={pageEl}
  onmousemove={move} onmouseleave={() => (selection.hover = "")} onmousedown={down} role="presentation"
  ondragover={dragover} ondragleave={() => (target = null)} ondrop={drop}>
  {#if visible}<canvas bind:this={canvas}></canvas>{/if}
  <div class="overlay">
    {#each frames as f}
      <div class="box frame" class:flowed={f.flowed} style={box(f.a.r)}>{#if f.flowed}<span class="flow">↓</span>{/if}</div>
    {/each}
    {#each hovered as a}
      <div class="box hover" data-hover={a.id} style={box(a.r)}></div>
    {/each}
    {#each selected as a}
      <div class="box related" style={box(a.r)}></div>
    {/each}
    {#if current}
      <div class="box sel" data-sel={selection.ref} style={box(current)}></div>
      {#if resizable && !drag?.active}
        {#each handles as [hx, hy]}
          <div class="handle" data-handle="{hx},{hy}" role="presentation"
            style="left:{(current[0] + hx * current[2]) * zoom}px;top:{(current[1] + hy * current[3]) * zoom}px;cursor:{cursor(hx, hy)}"
            onmousedown={(e) => startDrag(e, "resize", [hx, hy])}></div>
        {/each}
      {/if}
    {/if}
    {#if drag?.active}
      <div class="box ghost" data-ghost style={box(drag.live)}></div>
      {#each drag.guides as g}
        <div class="guide {g.axis}" style={g.axis === "x" ? `left:${g.at * zoom}px` : `top:${g.at * zoom}px`}></div>
      {/each}
      {#each spacing as s}
        <div class="gap" style="left:{Math.min(s.from[0], s.to[0]) * zoom}px;top:{Math.min(s.from[1], s.to[1]) * zoom}px;width:{Math.abs(s.to[0] - s.from[0]) * zoom}px;height:{Math.abs(s.to[1] - s.from[1]) * zoom}px">
          <span>{fmtMM(s.mm)}</span>
        </div>
      {/each}
      {#if drag.target}
        <div class="drop-line" data-reorder={drag.target.parent} style="left:{drag.target.marker.x * zoom}px;top:{drag.target.marker.y * zoom}px;width:{drag.target.marker.w * zoom}px"></div>
      {/if}
    {/if}
    {#if target}
      {#if target.positioned}
        <div class="drop-point" data-drop={target.parent} style="left:{target.marker.x * zoom}px;top:{target.marker.y * zoom}px"></div>
      {:else}
        <div class="drop-line" data-drop={target.parent} style="left:{target.marker.x * zoom}px;top:{target.marker.y * zoom}px;width:{target.marker.w * zoom}px"></div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .page {
    position: relative;
    background: #fff;
    box-shadow: var(--shadow);
    flex: none;
  }
  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }
  .overlay {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }
  .box {
    position: absolute;
    box-sizing: border-box;
  }
  .frame {
    outline: 1px dotted rgba(47, 111, 228, 0.45);
  }
  .flow {
    position: absolute;
    left: 2px;
    top: 0;
    font-size: 10px;
    color: rgba(47, 111, 228, 0.7);
  }
  .hover {
    outline: 1px solid var(--hover);
  }
  .related {
    outline: 1px dashed var(--accent);
  }
  .sel {
    outline: 2px solid var(--accent);
    background: var(--accent-soft);
  }
  .ghost {
    outline: 1px dashed var(--accent);
    background: var(--accent-soft);
  }
  .handle {
    position: absolute;
    width: 8px;
    height: 8px;
    margin: -4px 0 0 -4px;
    background: #fff;
    border: 1px solid var(--accent);
    box-sizing: border-box;
    pointer-events: auto;
  }
  .guide {
    position: absolute;
    background: #e5368d;
  }
  .guide.x {
    top: 0;
    bottom: 0;
    width: 1px;
  }
  .guide.y {
    left: 0;
    right: 0;
    height: 1px;
  }
  .gap {
    position: absolute;
    border: 0 dashed #e5368d;
    border-width: 1px 0 0 1px;
    min-width: 1px;
    min-height: 1px;
  }
  .gap span {
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    background: #e5368d;
    color: #fff;
    font-size: 10px;
    padding: 0 3px;
    border-radius: 3px;
    white-space: nowrap;
  }
  .drop-line {
    position: absolute;
    height: 0;
    border-top: 2px solid var(--accent);
    margin-top: -1px;
  }
  .drop-point {
    position: absolute;
    width: 10px;
    height: 10px;
    margin: -5px 0 0 -5px;
    border: 2px solid var(--accent);
    border-radius: 50%;
  }
</style>
