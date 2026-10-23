<script lang="ts">
  // Page overview: small images of all pages, a click jumps there.
  import type { Page } from "../../lib/canvas/model";
  import { drawPage, type ImageSource } from "../../lib/canvas/render";
  import { t } from "../../lib/i18n";
  import { render } from "../../lib/render.svelte";

  let { current, image, onpick }: { current: number; image: ImageSource; onpick: (i: number) => void } = $props();

  const width = 96;

  function draw(c: HTMLCanvasElement, page: Page) {
    const dpr = window.devicePixelRatio || 1;
    const scale = (width / page.w) * dpr;
    c.width = Math.ceil(page.w * scale);
    c.height = Math.ceil(page.h * scale);
    c.style.width = `${width}px`;
    c.style.height = `${(page.h / page.w) * width}px`;
    drawPage(c.getContext("2d")!, page, scale, image);
  }

  // Svelte action: redraw when the page changes.
  function thumb(c: HTMLCanvasElement, page: Page) {
    draw(c, page);
    return { update: (p: Page) => draw(c, p) };
  }
</script>

<nav class="thumbs" aria-label={t("canvas.pagesTitle")} data-testid="page-thumbs">
  {#each render.pages as p, i (p.n)}
    <button class:current={i === current} onclick={() => onpick(i)} aria-label={t("canvas.page", { n: p.n })} aria-current={i === current ? "page" : undefined}>
      {#key render.version}<canvas use:thumb={p}></canvas>{/key}
      <span>{p.n}</span>
    </button>
  {/each}
</nav>

<style>
  .thumbs {
    flex: none;
    width: 124px;
    overflow: auto;
    border-right: 1px solid var(--border);
    background: var(--surface);
    padding: var(--space-2) 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
  }
  button {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    border: 2px solid transparent;
    border-radius: 4px;
    background: none;
    padding: 2px;
  }
  button.current {
    border-color: var(--accent);
  }
  canvas {
    background: #fff;
    box-shadow: var(--shadow);
  }
  span {
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
