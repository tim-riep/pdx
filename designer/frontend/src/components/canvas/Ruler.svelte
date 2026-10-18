<script lang="ts">
  // Lineal entlang einer Seitenkante in mm oder pt. band markiert die
  // Ausdehnung der Auswahl (in pt von der Seitenkante).
  import { PT_PER_MM } from "../../lib/geometry";

  let {
    axis,
    length,
    zoom,
    unit,
    band = null,
  }: { axis: "x" | "y"; length: number; zoom: number; unit: "mm" | "pt"; band?: [number, number] | null } = $props();

  const size = 16;
  const unitPt = $derived(unit === "mm" ? PT_PER_MM : 1);
  const px = $derived(unitPt * zoom); // Pixel je Einheit
  const major = $derived([1, 2, 5, 10, 20, 50, 100, 200, 500, 1000].find((m) => m * px >= 36) ?? 1000);
  const minor = $derived([10, 5, 2, 1].map((d) => major / d).find((m) => m * px >= 4) ?? major);
  const ticks = $derived.by(() => {
    const out: { at: number; big: boolean; label: string }[] = [];
    const n = Math.floor(length / unitPt / minor);
    for (let i = 0; i <= n; i++) {
      const v = Math.round(i * minor * 1000) / 1000;
      const big = Math.abs(v / major - Math.round(v / major)) < 1e-6;
      out.push({ at: v * px, big, label: big && v > 0 ? String(v) : "" });
    }
    return out;
  });
  const total = $derived(length * zoom);
</script>

<svg class="ruler {axis}" data-ruler={axis} width={axis === "x" ? total : size} height={axis === "x" ? size : total} aria-hidden="true">
  {#if band}
    {#if axis === "x"}<rect class="band" x={band[0] * zoom} y="0" width={(band[1] - band[0]) * zoom} height={size} />
    {:else}<rect class="band" x="0" y={band[0] * zoom} width={size} height={(band[1] - band[0]) * zoom} />{/if}
  {/if}
  {#each ticks as tk}
    {#if axis === "x"}
      <line x1={tk.at} x2={tk.at} y1={size} y2={size - (tk.big ? 8 : 3)} />
      {#if tk.label}<text x={tk.at + 2} y="9">{tk.label}</text>{/if}
    {:else}
      <line y1={tk.at} y2={tk.at} x1={size} x2={size - (tk.big ? 8 : 3)} />
      {#if tk.label}<text x="1" y={tk.at + 9}>{tk.label}</text>{/if}
    {/if}
  {/each}
</svg>

<style>
  .ruler {
    position: absolute;
    background: var(--surface);
    pointer-events: none;
  }
  .ruler.x {
    left: 0;
    top: -18px;
    border-bottom: 1px solid var(--border);
  }
  .ruler.y {
    top: 0;
    left: -18px;
    border-right: 1px solid var(--border);
  }
  line {
    stroke: var(--text-muted);
    stroke-width: 1;
  }
  text {
    font: 8px var(--font-ui);
    fill: var(--text-muted);
  }
  .band {
    fill: var(--accent-soft);
  }
</style>
