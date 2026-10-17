<script lang="ts">
  import type { Snippet } from "svelte";
  let { tabs, active = $bindable(), children }: { tabs: [string, string][]; active: string; children: Snippet } = $props();
</script>

<div class="tabs" role="tablist">
  {#each tabs as [key, label]}
    <button role="tab" aria-selected={active === key} class:active={active === key} onclick={() => (active = key)}>{label}</button>
  {/each}
</div>
<div class="tab-body">{@render children()}</div>

<style>
  .tabs {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--border);
    padding: 0 var(--space-2);
    flex: none;
  }
  .tabs button {
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    background: none;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    padding: var(--space-2) var(--space-2);
  }
  .tabs button.active {
    color: var(--text);
    border-bottom-color: var(--accent);
  }
  .tab-body {
    flex: 1;
    overflow: auto;
    min-height: 0;
  }
</style>
