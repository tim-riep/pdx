<script lang="ts">
  import { closeMenu, menu } from "../lib/menu.svelte";

  let el: HTMLDivElement | undefined = $state();
  let index = $state(-1);

  // Keep inside the window.
  const pos = $derived.by(() => {
    const w = el?.offsetWidth ?? 220;
    const h = el?.offsetHeight ?? 300;
    return { x: Math.min(menu.x, window.innerWidth - w - 8), y: Math.min(menu.y, window.innerHeight - h - 8) };
  });

  $effect(() => {
    if (menu.open) {
      index = -1;
      queueMicrotask(() => el?.focus());
    }
  });

  function pick(i: number) {
    const it = menu.items[i];
    if (!it || it.disabled) return;
    closeMenu();
    it.action();
  }

  function onkeydown(e: KeyboardEvent) {
    const enabled = menu.items.map((it, i) => (it.disabled ? -1 : i)).filter((i) => i >= 0);
    if (e.key === "Escape") closeMenu();
    else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      const at = enabled.indexOf(index);
      const next = e.key === "ArrowDown" ? enabled[(at + 1) % enabled.length] : enabled[(at - 1 + enabled.length) % enabled.length];
      index = next ?? -1;
    } else if (e.key === "Enter" && index >= 0) pick(index);
    else return;
    e.preventDefault();
    e.stopPropagation();
  }
</script>

{#if menu.open}
  <div class="backdrop" role="presentation" onmousedown={closeMenu} oncontextmenu={(e) => { e.preventDefault(); closeMenu(); }}></div>
  <div bind:this={el} class="menu" role="menu" tabindex="-1" style:left="{pos.x}px" style:top="{pos.y}px" {onkeydown} data-testid="context-menu">
    {#each menu.items as it, i (i)}
      {#if it.section}<div class="section">{it.section}</div>{/if}
      <button role="menuitem" class:active={i === index} disabled={it.disabled} data-testid={it.testid}
        onmouseenter={() => (index = i)} onclick={() => pick(i)}>
        <span>{it.label}</span>
        {#if it.shortcut}<kbd>{it.shortcut}</kbd>{/if}
      </button>
    {/each}
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 90;
  }
  .menu {
    position: fixed;
    z-index: 91;
    min-width: 200px;
    max-height: 70vh;
    overflow: auto;
    padding: 4px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    outline: none;
  }
  .section {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    padding: 6px 8px 2px;
  }
  button {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
    border: 0;
    background: none;
    text-align: left;
    padding: 3px 8px;
    border-radius: 3px;
  }
  button.active:not(:disabled) {
    background: var(--accent-soft);
  }
  button:disabled {
    color: var(--text-muted);
  }
  kbd {
    font-family: var(--font-ui);
    color: var(--text-muted);
    font-size: 11px;
  }
</style>
