<script lang="ts">
  import type { Doc, Element, Page } from "../core/model";
  import type { PageCache } from "../core/render";

  let {
    doc, page, cache, zoom, visible, hover, sel, related, drag,
    onpointer,
  }: {
    doc: Doc; page: Page; cache: PageCache; zoom: number; visible: boolean;
    hover: Element | null; sel: Element | null; related: Element[];
    drag: [number, number] | null;
    onpointer: (type: string, page: Page, x: number, y: number) => void;
  } = $props();

  let canvas: HTMLCanvasElement | undefined = $state();

  $effect(() => {
    if (!visible || !canvas) return;
    const dpr = window.devicePixelRatio || 1;
    const img = cache.get(page, zoom * dpr);
    canvas.width = img.width;
    canvas.height = img.height;
    canvas.getContext("2d")!.drawImage(img, 0, 0);
  });

  function handler(e: MouseEvent) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    onpointer(e.type, page, (e.clientX - r.left) / zoom, (e.clientY - r.top) / zoom);
  }

  const box = (r: number[], dx = 0, dy = 0) =>
    `left:${(r[0] + dx) * zoom}px;top:${(r[1] + dy) * zoom}px;width:${r[2] * zoom}px;height:${r[3] * zoom}px`;
  const corners = [[0, 0], [0.5, 0], [1, 0], [1, 0.5], [1, 1], [0.5, 1], [0, 1], [0, 0.5]];
</script>

<div class="page" data-page={page.n} style="width:{page.w * zoom}px;height:{page.h * zoom}px"
  onmousemove={handler} onmousedown={handler} onmouseup={handler} role="presentation">
  {#if visible}
    <canvas bind:this={canvas}></canvas>
  {/if}
  <div class="overlay">
    {#each related as r}
      <div class="box related" style={box(r.r)}></div>
    {/each}
    {#if hover}
      <div class="box hover" data-hover={hover.id} style={box(hover.r)}></div>
    {/if}
    {#if sel}
      <div class="box sel" data-sel={sel.id} style={box(sel.r)}></div>
      {#each corners as [fx, fy]}
        <div class="handle" style="left:{(sel.r[0] + fx * sel.r[2]) * zoom}px;top:{(sel.r[1] + fy * sel.r[3]) * zoom}px"></div>
      {/each}
      {#if drag}
        <div class="box ghost" data-drag="{drag[0]},{drag[1]}" style={box(sel.r, drag[0], drag[1])}></div>
      {/if}
    {/if}
  </div>
</div>
