<script lang="ts">
  import type { Area, Page } from "../../lib/canvas/model";
  import { drawPage, type ImageSource } from "../../lib/canvas/render";
  import { HitIndex } from "../../lib/canvas/hit";
  import { select, selection } from "../../lib/selection.svelte";

  let { page, hash, zoom, visible, bitmap, image }: {
    page: Page; hash: string; zoom: number; visible: boolean;
    bitmap: (p: Page, key: string, scale: number, draw: (c: HTMLCanvasElement) => void) => HTMLCanvasElement;
    image: ImageSource;
  } = $props();

  let canvas: HTMLCanvasElement | undefined = $state();
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

  // Bereiche des ausgewählten und des überfahrenen Elements auf dieser Seite.
  const selected = $derived(selection.ref ? page.els.filter((a) => a.id === selection.ref) : []);
  const hovered = $derived(
    selection.hover && selection.hover !== selection.ref ? page.els.filter((a) => a.id === selection.hover) : [],
  );
  const current = $derived(
    selection.instance?.page === page.n ? selection.instance.r : selected.length ? selected[0].r : null,
  );

  function point(e: MouseEvent): [number, number] {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    return [(e.clientX - r.left) / zoom, (e.clientY - r.top) / zoom];
  }

  function move(e: MouseEvent) {
    const hit = index.at(...point(e));
    const id = hit?.id ?? "";
    if (id !== selection.hover) selection.hover = id;
  }

  function down(e: MouseEvent) {
    if (e.button !== 0) return;
    const hit: Area | undefined = index.at(...point(e));
    if (hit) select(hit.id, { page: page.n, r: hit.r });
    else select("");
  }

  const box = (r: number[]) =>
    `left:${r[0] * zoom}px;top:${r[1] * zoom}px;width:${r[2] * zoom}px;height:${r[3] * zoom}px`;
</script>

<div class="page" data-page={page.n} style="width:{page.w * zoom}px;height:{page.h * zoom}px"
  onmousemove={move} onmouseleave={() => (selection.hover = "")} onmousedown={down} role="presentation">
  {#if visible}<canvas bind:this={canvas}></canvas>{/if}
  <div class="overlay">
    {#each hovered as a}
      <div class="box hover" data-hover={a.id} style={box(a.r)}></div>
    {/each}
    {#each selected as a}
      <div class="box related" style={box(a.r)}></div>
    {/each}
    {#if current}
      <div class="box sel" data-sel={selection.ref} style={box(current)}></div>
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
</style>
