<script lang="ts">
  import { t } from "../lib/i18n";
  import { closePalette, match, type PaletteItem } from "../lib/palette.svelte";

  let { items }: { items: () => PaletteItem[] } = $props();

  let query = $state("");
  let index = $state(0);
  let input: HTMLInputElement | undefined = $state();
  const all = $derived(items());
  const list = $derived(
    all
      .map((it, i) => ({ it, score: match(it, query), i }))
      .filter((x) => x.score > 0)
      .sort((a, b) => b.score - a.score || a.i - b.i)
      .slice(0, 50)
      .map((x) => x.it),
  );

  $effect(() => {
    input?.focus();
  });
  $effect(() => {
    void query;
    index = 0;
  });

  function run(it: PaletteItem | undefined) {
    if (!it) return;
    closePalette();
    it.run();
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      index = (index + (e.key === "ArrowDown" ? 1 : list.length - 1)) % Math.max(1, list.length);
      document.querySelector(`[data-palette-index="${index}"]`)?.scrollIntoView({ block: "nearest" });
    } else if (e.key === "Enter") run(list[index]);
    else if (e.key === "Escape") closePalette();
    else return;
    e.preventDefault();
    e.stopPropagation();
  }
</script>

<div class="backdrop" role="presentation" onmousedown={closePalette}></div>
<div class="palette" role="dialog" aria-label={t("palette.label")} data-testid="command-palette">
  <input bind:this={input} bind:value={query} placeholder={t("palette.placeholder")} aria-label={t("palette.placeholder")} spellcheck="false"
    role="combobox" aria-expanded="true" aria-controls="palette-list" {onkeydown} />
  <ul id="palette-list" role="listbox">
    {#each list as it, i (it.id)}
      <li role="option" aria-selected={i === index} class:sel={i === index} data-palette-index={i}
        onmouseenter={() => (index = i)} onmousedown={(e) => { e.preventDefault(); run(it); }}>
        <span class="group">{it.group}</span>
        <span class="label">{it.label}{#if it.hint}<code>{it.hint}</code>{/if}</span>
        {#if it.shortcut}<kbd>{it.shortcut}</kbd>{/if}
      </li>
    {:else}
      <li class="none">{t("palette.none")}</li>
    {/each}
  </ul>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 110;
    background: rgba(0, 0, 0, 0.15);
  }
  .palette {
    position: fixed;
    z-index: 111;
    top: 12vh;
    left: 50%;
    transform: translateX(-50%);
    width: min(620px, 90vw);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.3);
    overflow: hidden;
  }
  input {
    width: 100%;
    box-sizing: border-box;
    border: 0;
    border-bottom: 1px solid var(--border);
    padding: var(--space-3);
    font-size: 15px;
    outline: none;
    background: var(--surface);
    color: var(--text);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 4px;
    max-height: 50vh;
    overflow: auto;
  }
  li {
    display: grid;
    grid-template-columns: 90px 1fr auto;
    gap: var(--space-2);
    align-items: baseline;
    padding: 4px 8px;
    border-radius: 4px;
    cursor: pointer;
  }
  li.sel {
    background: var(--accent-soft);
  }
  li.none {
    display: block;
    color: var(--text-muted);
    cursor: default;
  }
  .group {
    color: var(--text-muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  code {
    margin-left: 8px;
    font-size: 11px;
    color: var(--text-muted);
  }
  kbd {
    font-family: var(--font-ui);
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
