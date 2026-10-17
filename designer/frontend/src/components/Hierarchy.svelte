<script lang="ts">
  import { t, label } from "../lib/i18n";
  import { node, path, tpl } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";

  let open: Record<string, boolean> = $state({});

  // Vorfahren der Auswahl aufklappen.
  $effect(() => {
    if (selection.ref && tpl.snapshot) for (const r of path(selection.ref).slice(0, -1)) open[r] = true;
  });

  const kids = (ref: string) => (node(ref)?.children ?? []).filter((c) => c.ref).map((c) => c.ref!);
</script>

{#snippet tree(ref: string, depth: number)}
  {@const n = node(ref)}
  {#if n}
    {@const children = kids(ref)}
    <div class="row" class:selected={selection.ref === ref} class:hover={selection.hover === ref}
      style="padding-left: {depth * 12 + 4}px" role="treeitem" tabindex="-1" aria-selected={selection.ref === ref}
      aria-expanded={children.length ? !!open[ref] : undefined}
      onmouseenter={() => (selection.hover = ref)} onmouseleave={() => (selection.hover = "")}>
      {#if children.length}
        <button class="twisty" aria-label={open[ref] ? t("tree.collapse") : t("tree.expand")} onclick={() => (open[ref] = !open[ref])}>{open[ref] ? "▾" : "▸"}</button>
      {:else}<span class="twisty"></span>{/if}
      <button class="name" onclick={() => select(ref, null, true)}>
        <span class="type">{label(n.type, n.name)}</span>
        {#if !ref.startsWith("~")}<span class="id">{ref}</span>{/if}
      </button>
    </div>
    {#if open[ref]}
      {#each children as c (c)}{@render tree(c, depth + 1)}{/each}
    {/if}
  {/if}
{/snippet}

<div class="hierarchy" role="tree" aria-label={t("panel.hierarchy")} data-testid="hierarchy">
  {#if tpl.snapshot}
    {#each kids(tpl.snapshot.root) as r (r)}{@render tree(r, 0)}{/each}
  {/if}
</div>

<style>
  .hierarchy {
    padding: var(--space-1) 0;
  }
  .row {
    display: flex;
    align-items: center;
    border-radius: 4px;
    margin: 0 var(--space-1);
  }
  .row.hover {
    background: var(--surface-2);
  }
  .row.selected {
    background: var(--accent-soft);
  }
  button {
    border: 0;
    background: none;
    padding: 2px 4px;
  }
  .twisty {
    width: 16px;
    display: inline-block;
    color: var(--text-muted);
    padding: 0;
  }
  .name {
    display: flex;
    gap: var(--space-2);
    flex: 1;
    text-align: left;
    overflow: hidden;
    white-space: nowrap;
  }
  .id {
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
