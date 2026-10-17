<script lang="ts">
  import type { Area, Page } from "../../lib/canvas/model";
  import { drawPage, type ImageSource } from "../../lib/canvas/render";
  import { HitIndex } from "../../lib/canvas/hit";
  import { select, selection } from "../../lib/selection.svelte";
  import { DATA_MIME } from "../../lib/data.svelte";
  import { dropTarget, type Target } from "../../lib/drop";
  import { node, tpl, typeOf } from "../../lib/template.svelte";
  import { TemplateAPI, message } from "../../lib/api";
  import { ui } from "../../lib/project.svelte";

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

  // Ablage gezogener Datenknoten.
  let target: Target | null = $state(null);
  const model = {
    node,
    type: (n: import("../../lib/api").ViewNode) => typeOf(n),
    body: () => Object.values(tpl.snapshot?.nodes ?? {}).find((n) => n?.name === "body"),
  };

  function dragover(e: DragEvent) {
    if (!e.dataTransfer?.types.includes(DATA_MIME)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
    target = dropTarget(model, page, index, ...point(e));
  }

  async function drop(e: DragEvent) {
    const path = e.dataTransfer?.getData(DATA_MIME);
    const tg = target;
    target = null;
    if (!path || !tg) return;
    e.preventDefault();
    try {
      const res = await TemplateAPI.Drop({ path, parent: tg.parent, index: tg.index, x: tg.x, y: tg.y });
      tpl.history = res.history;
      if (res.select) select(res.select);
      ui.error = "";
    } catch (err) {
      ui.error = message(err);
    }
  }

  const box = (r: number[]) =>
    `left:${r[0] * zoom}px;top:${r[1] * zoom}px;width:${r[2] * zoom}px;height:${r[3] * zoom}px`;
</script>

<div class="page" data-page={page.n} style="width:{page.w * zoom}px;height:{page.h * zoom}px"
  onmousemove={move} onmouseleave={() => (selection.hover = "")} onmousedown={down} role="presentation"
  ondragover={dragover} ondragleave={() => (target = null)} ondrop={drop}>
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
