<script lang="ts">
  import { t, label } from "../lib/i18n";
  import { node, path, tpl } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";
  import { openElementMenu } from "../lib/menu.svelte";

  let open: Record<string, boolean> = $state({});

  // Die Wurzel ist immer aufgeklappt, sie nimmt Formate, Texte und Anhänge auf.
  $effect(() => {
    const root = tpl.snapshot?.root;
    if (root && open[root] === undefined) open[root] = true;
  });

  function menuAt(e: MouseEvent, ref: string) {
    e.preventDefault();
    select(ref);
    openElementMenu(ref, e.clientX, e.clientY);
  }

  function menuButton(e: MouseEvent, ref: string) {
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    openElementMenu(ref, r.left, r.bottom + 2);
  }

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
      onmouseenter={() => (selection.hover = ref)} onmouseleave={() => (selection.hover = "")} oncontextmenu={(e) => menuAt(e, ref)}>
      {#if children.length}
        <button class="twisty" aria-label={open[ref] ? t("tree.collapse") : t("tree.expand")} onclick={() => (open[ref] = !open[ref])}>{open[ref] ? "▾" : "▸"}</button>
      {:else}<span class="twisty"></span>{/if}
      <button class="name" onclick={() => select(ref, null, true)}>
        <span class="type">{label(n.type, n.name)}</span>
        {#if !ref.startsWith("~")}<span class="id">{ref}</span>{/if}
      </button>
      {#if selection.ref === ref}
        <button class="more" aria-label={t("menu.open")} title={t("menu.open")} onclick={(e) => menuButton(e, ref)}>⋯</button>
      {/if}
    </div>
    {#if open[ref]}
      {#each children as c (c)}{@render tree(c, depth + 1)}{/each}
    {/if}
  {/if}
{/snippet}

<div class="hierarchy" role="tree" aria-label={t("panel.hierarchy")} data-testid="hierarchy">
  {#if tpl.snapshot}
    {@render tree(tpl.snapshot.root, 0)}
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
  .more {
    color: var(--text-muted);
    padding: 0 6px;
  }
  .id {
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
