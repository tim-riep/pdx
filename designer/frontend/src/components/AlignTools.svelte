<script lang="ts">
  import type { ViewNode } from "../lib/api";
  import { t } from "../lib/i18n";
  import { apply, isPositioned, node } from "../lib/template.svelte";
  import { selection } from "../lib/selection.svelte";
  import { render } from "../lib/render.svelte";
  import { PT_PER_MM, toMM } from "../lib/geometry";
  import { fmtLen } from "../lib/units.svelte";
  import { align, distribute, type AlignKind, type Box, type Move } from "../lib/align";

  let { n }: { n: ViewNode } = $props();

  // Erster Bereich eines Elements im Satz, in Punkt.
  function area(ref: string): number[] | null {
    for (const p of render.pages) {
      const a = p.els.find((e) => e.id === ref);
      if (a) return a.r;
    }
    return null;
  }

  // boxOf liest Lage aus der Vorlage und Größe aus der Vorlage oder dem Satz.
  function boxOf(ref: string): Box | null {
    const x = node(ref);
    if (!x) return null;
    const r = area(ref);
    const px = toMM(x.attrs.x ?? "0mm"), py = toMM(x.attrs.y ?? "0mm");
    const w = toMM(x.attrs.w) ?? (r ? r[2] / PT_PER_MM : null);
    const h = toMM(x.attrs.h) ?? (r ? r[3] / PT_PER_MM : null);
    if (px === null || py === null || w === null || h === null) return null;
    return { ref, x: px, y: py, w, h };
  }

  function parentSize(): { w: number; h: number } {
    const p = node(n.parent);
    const r = p ? area(p.ref) : null;
    const pageW = render.pages[0]?.w ?? 595;
    return {
      w: toMM(p?.attrs.w) ?? (r ? r[2] / PT_PER_MM : pageW / PT_PER_MM),
      h: toMM(p?.attrs.h ?? p?.attrs.height) ?? (r ? r[3] / PT_PER_MM : 0),
    };
  }

  const refs = $derived([n.ref, ...selection.extra]);
  const boxes = $derived(refs.map(boxOf).filter((b): b is Box => !!b));

  async function run(label: string, moves: Move[]) {
    const cmds = [];
    for (const m of moves) {
      if (m.x !== undefined) cmds.push({ op: "setAttr", ref: m.ref, name: "x", value: fmtLen(Math.max(0, m.x)) });
      if (m.y !== undefined) cmds.push({ op: "setAttr", ref: m.ref, name: "y", value: fmtLen(Math.max(0, m.y)) });
    }
    if (cmds.length) await apply(label, { op: "batch", cmds: cmds as never });
  }

  const kinds: [AlignKind, string][] = [
    ["left", "⇤"], ["hcenter", "↔"], ["right", "⇥"], ["top", "⤒"], ["vcenter", "↕"], ["bottom", "⤓"],
  ];
</script>

{#if isPositioned(n)}
  <section class="align" data-testid="align-tools">
    <h3>{t("align.title")}</h3>
    <div class="buttons">
      {#each kinds as [k, icon] (k)}
        <button title={t(`align.${k}`)} aria-label={t(`align.${k}`)} disabled={!boxes.length}
          onclick={() => run(t("cmd.align"), align(boxes, k, parentSize()))}>{icon}</button>
      {/each}
      {#if refs.length >= 3}
        <button title={t("align.distH")} aria-label={t("align.distH")} onclick={() => run(t("cmd.distribute"), distribute(boxes, "x"))}>⋯</button>
        <button title={t("align.distV")} aria-label={t("align.distV")} onclick={() => run(t("cmd.distribute"), distribute(boxes, "y"))}>⋮</button>
      {/if}
    </div>
    <p class="hint">{refs.length > 1 ? t("align.hintMany", { n: refs.length }) : t("align.hintOne")}</p>
  </section>
{/if}

<style>
  .align {
    margin-bottom: var(--space-2);
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: 0 0 var(--space-1);
  }
  .buttons {
    display: flex;
    gap: 2px;
    flex-wrap: wrap;
  }
  button {
    width: 28px;
    height: 24px;
    padding: 0;
    font-size: 14px;
  }
  .hint {
    margin: 2px 0 0;
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
