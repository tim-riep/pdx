<script lang="ts">
  import type { Page } from "../../lib/canvas/model";
  import { drawPage, type ImageSource } from "../../lib/canvas/render";

  let { page, hash, zoom, visible, bitmap, image }: {
    page: Page; hash: string; zoom: number; visible: boolean;
    bitmap: (p: Page, key: string, scale: number, draw: (c: HTMLCanvasElement) => void) => HTMLCanvasElement;
    image: ImageSource;
  } = $props();

  let canvas: HTMLCanvasElement | undefined = $state();

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
</script>

<div class="page" data-page={page.n} style="width:{page.w * zoom}px;height:{page.h * zoom}px">
  {#if visible}<canvas bind:this={canvas}></canvas>{/if}
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
</style>
