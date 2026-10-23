<script lang="ts">
  import type { Page } from "../../lib/canvas/model";
  import { image, render } from "../../lib/render.svelte";
  import { selection } from "../../lib/selection.svelte";
  import { t } from "../../lib/i18n";
  import PageView from "./PageView.svelte";
  import Palette from "./Palette.svelte";
  import PageThumbs from "./PageThumbs.svelte";
  import { view } from "../../lib/units.svelte";

  const gap = 24;
  let zoom = $state(1);
  let grid = $state(true);
  let fit = $state(true);
  let scrollTop = $state(0);
  let height = $state(800);
  let width = $state(800);

  // Bitmaps per page and magnification, so scrolling only copies.
  const bitmaps = new Map<string, HTMLCanvasElement>();

  // Fit the width as long as nobody has zoomed by hand.
  $effect(() => {
    if (fit && render.pages.length) {
      const w = Math.max(...render.pages.map((p) => p.w));
      zoom = Math.max(0.25, Math.min(3, (width - 2 * gap) / w));
    }
  });

  const tops = $derived.by(() => {
    let y = gap;
    return render.pages.map((p) => {
      const top = y;
      y += p.h * zoom + gap;
      return top;
    });
  });

  // The page that fills the upper part of the area.
  const currentPage = $derived.by(() => {
    let cur = 0;
    for (let i = 0; i < tops.length; i++) if (tops[i] <= scrollTop + height / 3) cur = i;
    return cur;
  });

  function visible(i: number): boolean {
    const top = tops[i], bottom = top + render.pages[i].h * zoom;
    return bottom > scrollTop - height && top < scrollTop + 2 * height;
  }

  let viewport: HTMLElement | undefined = $state();

  // Make the selected element visible, for example after a click in the hierarchy.
  // Only once per request: otherwise the view jumps on every layout run and every
  // later selection.
  let revealed = 0;
  $effect(() => {
    const req = selection.reveal;
    if (!req || req === revealed || !viewport) return;
    const ref = selection.ref;
    for (let i = 0; i < render.pages.length; i++) {
      const a = render.pages[i].els.find((e) => e.id === ref);
      if (a) {
        revealed = req;
        const y = tops[i] + a.r[1] * zoom - 48;
        if (y < viewport.scrollTop || y > viewport.scrollTop + height - 96) viewport.scrollTo({ top: y });
        break;
      }
    }
  });

  function setZoom(z: number) {
    fit = false;
    zoom = Math.max(0.25, Math.min(4, Math.round(z * 100) / 100));
  }

  function wheel(e: WheelEvent) {
    if (!(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    setZoom(zoom * Math.exp(-e.deltaY / 300));
  }

  export function bitmap(p: Page, key: string, scale: number, draw: (c: HTMLCanvasElement) => void) {
    const k = `${key}@${scale}@${render.version}`;
    let c = bitmaps.get(k);
    if (!c) {
      c = document.createElement("canvas");
      draw(c);
      bitmaps.set(k, c);
      if (bitmaps.size > 60) bitmaps.delete(bitmaps.keys().next().value!);
    }
    return c;
  }
</script>

<div class="canvas-tools">
  {#if render.view === "design"}<Palette /><span class="sep"></span>{/if}
  <label class="grid"><input type="checkbox" bind:checked={grid} /> {t("canvas.grid")}</label>
  <label class="grid"><input type="checkbox" bind:checked={view.rulers} /> {t("canvas.rulers")}</label>
  <label class="grid"><input type="checkbox" bind:checked={view.pages} /> {t("canvas.pages")}</label>
  <select aria-label={t("canvas.unit")} bind:value={view.unit}><option value="mm">mm</option><option value="pt">pt</option></select>
  <button onclick={() => setZoom(zoom / 1.25)} aria-label={t("canvas.zoomOut")}>−</button>
  <span data-testid="zoom">{Math.round(zoom * 100)} %</span>
  <button onclick={() => setZoom(zoom * 1.25)} aria-label={t("canvas.zoomIn")}>+</button>
  <button onclick={() => (fit = true)}>{t("canvas.fit")}</button>
  {#if render.manifest}
    <span class="info" data-testid="render-info">{t(render.pages.length === 1 ? "canvas.infoOne" : "canvas.info", { pages: render.pages.length, ms: render.manifest.millis })}</span>
  {/if}
</div>
<div class="body">
{#if view.pages}<PageThumbs current={currentPage} {image} onpick={(i) => viewport?.scrollTo({ top: tops[i] - gap })} />{/if}
<div class="viewport" data-viewport bind:this={viewport} bind:clientHeight={height} bind:clientWidth={width} onwheel={wheel}
  onscroll={(e) => (scrollTop = (e.currentTarget as HTMLElement).scrollTop)}>
  <div class="pages" style="padding: {gap}px; gap: {gap}px">
    {#each render.pages as p, i (p.n)}
      <PageView page={p} hash={render.manifest?.pages[i]?.hash ?? String(p.n)} {zoom} {grid} visible={visible(i)} {bitmap} {image} />
    {/each}
  </div>
</div>
</div>

<style>
  .body {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .canvas-tools {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .sep {
    width: 1px;
    align-self: stretch;
    background: var(--border);
  }
  .grid {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--text-muted);
  }
  .canvas-tools .info {
    margin-left: auto;
    color: var(--text-muted);
    font-size: 12px;
  }
  .viewport {
    flex: 1;
    overflow: auto;
    position: relative;
  }
  .pages {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: min-content;
  }
</style>
