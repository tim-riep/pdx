<script lang="ts">
  import type { Doc, Element, Page as PageT } from "../core/model";
  import { PageCache } from "../core/render";
  import { HitIndex } from "../core/hit";
  import Page from "./Page.svelte";

  let { doc }: { doc: Doc } = $props();

  // The area tree does not change in the prototype, so the initial value is enough.
  // svelte-ignore state_referenced_locally
  const cache = new PageCache(doc);
  // svelte-ignore state_referenced_locally
  const index = new Map(doc.pages.map((p) => [p.n, new HitIndex(p)]));

  let zoom = $state(1);
  let scrollTop = $state(0);
  let height = $state(800);
  let hover: { n: number; el: Element } | null = $state.raw(null);
  let sel: { n: number; el: Element } | null = $state.raw(null);
  let drag: { n: number; x: number; y: number; dx: number; dy: number; active: boolean } | null = $state(null);
  let last = $state("");

  window.__app = { setZoom: (z) => (zoom = z), zoom: () => zoom };

  // Visible pages from the scroll position, with one page of lookahead.
  const tops = $derived.by(() => {
    let y = 24;
    return doc.pages.map((p) => {
      const t = y;
      y += p.h * zoom + 24;
      return t;
    });
  });
  function isVisible(i: number): boolean {
    const p = doc.pages[i];
    const t = tops[i], b = t + p.h * zoom;
    return b > scrollTop - height && t < scrollTop + 2 * height;
  }

  function onpointer(type: string, page: PageT, x: number, y: number) {
    const hit = index.get(page.n)!.at(x, y) ?? null;
    if (type === "mousemove") {
      if (drag) {
        drag.dx = Math.round(x - drag.x);
        drag.dy = Math.round(y - drag.y);
        drag.active = true;
        return;
      }
      if (hit !== (hover?.el ?? null)) hover = hit ? { n: page.n, el: hit } : null;
    } else if (type === "mousedown") {
      if (sel && hit && sel.n === page.n && hit === sel.el) {
        drag = { n: page.n, x, y, dx: 0, dy: 0, active: false };
      } else {
        sel = hit ? { n: page.n, el: hit } : null;
      }
    } else if (type === "mouseup" && drag) {
      if (drag.dx || drag.dy) last = `${sel?.el.id} verschieben um ${drag.dx}, ${drag.dy} pt`;
      drag = null;
    }
  }

  const related = $derived.by(() => {
    const out = new Map<number, Element[]>();
    if (!sel) return out;
    for (const p of doc.pages) out.set(p.n, p.els.filter((e) => e.id === sel!.el.id && e !== sel!.el));
    return out;
  });
</script>

<div class="shell">
  <div class="toolbar">
    <strong>Svelte</strong>
    <button onclick={() => (zoom = Math.max(0.25, zoom / 1.25))}>−</button>
    <span>{Math.round(zoom * 100)} %</span>
    <button onclick={() => (zoom = Math.min(4, zoom * 1.25))}>+</button>
    <span class="info">{sel ? `Auswahl: ${sel.el.id}` : "keine Auswahl"} {last}</span>
  </div>
  <div class="viewport" data-viewport bind:clientHeight={height}
    onscroll={(e) => (scrollTop = (e.currentTarget as HTMLElement).scrollTop)}>
    <div class="pages">
      {#each doc.pages as p, i (p.n)}
        <Page {doc} page={p} {cache} {zoom} visible={isVisible(i)}
          hover={hover?.n === p.n ? hover.el : null}
          sel={sel?.n === p.n ? sel.el : null}
          related={related.get(p.n) ?? []}
          drag={drag?.active && drag.n === p.n ? [drag.dx, drag.dy] : null}
          {onpointer} />
      {/each}
    </div>
  </div>
</div>
