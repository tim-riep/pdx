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
    textField: "M1.5 5h13v6h-13zM4 6.5v3",
    checkBox: "M3 3h10v10H3zM5 8l2 2 4-4",
    radioGroup: "M8 3a5 5 0 1 0 0.01 0zM8 6a2 2 0 1 0 0.01 0z",
    comboBox: "M1.5 5h13v6h-13zM10.5 5v6M11.5 7l1 1.5 1-1.5",
    listBox: "M2 2h12v12H2zM4 5h8M4 8h8M4 11h8",
    signatureField: "M2 12h12M3 10c2-5 3-5 3-1s2 1 3-1 2 2 4 0",
  };

  function dragstart(e: DragEvent, kind: Kind) {
    e.dataTransfer?.setData(ELEMENT_MIME, kind);
    e.dataTransfer?.setData("text/plain", kind);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "copy";
  }
</script>

<div class="palette" role="toolbar" aria-label={t("insert.palette")} data-testid="palette">
  {#each kinds as k}
    {#if k === "textField"}<span class="sep" aria-hidden="true"></span>{/if}
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
    align-items: center;
  }
  .sep {
    width: 1px;
    height: 16px;
    background: var(--border);
    margin: 0 4px;
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
