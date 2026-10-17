<script lang="ts">
  import { t } from "../../lib/i18n";
  import { ELEMENT_MIME, insertAtSelection, kinds, type Kind } from "../../lib/insert";

  // Einfache Symbole als SVG-Pfade, 16 mal 16.
  const icons: Record<Kind, string> = {
    text: "M3 3h10M8 3v10M6 13h4",
    positioned: "M2 2h12v12H2zM5 5h4v3H5z",
    flowed: "M2 2h12v12H2zM5 5h6M5 8h6M5 11h4",
    line: "M2 13L14 3",
    rect: "M2 4h12v8H2z",
    image: "M2 3h12v10H2zM2 11l4-4 3 3 2-2 3 3",
    barcode: "M3 3v10M5 3v10M8 3v10M9 3v10M12 3v10M13 3v10",
    pageNumber: "M4 3h8v10H4zM7 7l1-1v5",
    table: "M2 3h12v10H2zM2 6h12M2 9.5h12M6 3v10M10 3v10",
  };

  function dragstart(e: DragEvent, kind: Kind) {
    e.dataTransfer?.setData(ELEMENT_MIME, kind);
    e.dataTransfer?.setData("text/plain", kind);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "copy";
  }
</script>

<div class="palette" role="toolbar" aria-label={t("insert.palette")} data-testid="palette">
  {#each kinds as k}
    <button draggable="true" title={t(`insert.${k}`)} aria-label={t(`insert.${k}`)} data-kind={k}
      ondragstart={(e) => dragstart(e, k)} onclick={() => insertAtSelection(k)}>
      <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d={icons[k]} /></svg>
    </button>
  {/each}
</div>

<style>
  .palette {
    display: flex;
    gap: 2px;
  }
  button {
    border: 1px solid transparent;
    background: none;
    padding: 3px 5px;
    cursor: grab;
    line-height: 0;
  }
  button:hover {
    border-color: var(--border);
    background: var(--surface-2);
  }
  path {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.3;
    stroke-linejoin: round;
  }
</style>
