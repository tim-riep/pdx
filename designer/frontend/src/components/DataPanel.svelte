<script lang="ts">
  import type { DataNode } from "../lib/api";
  import { t } from "../lib/i18n";
  import { DATA_MIME, cardinality, data, kids, loadSchema, roots, typeKey } from "../lib/data.svelte";
  import { askPath } from "../lib/project.svelte";

  let query = $state("");
  let open: Record<string, boolean> = $state({});

  const icons: Record<string, string> = { string: "abc", decimal: "1,0", integer: "123", date: "▦", dateTime: "◷", boolean: "✓", complex: "{ }" };

  function matches(n: DataNode, q: string): boolean {
    return n.name.toLowerCase().includes(q) || kids(n).some((c) => matches(c, q));
  }

  function dragstart(e: DragEvent, n: DataNode) {
    e.dataTransfer?.setData(DATA_MIME, n.path);
    e.dataTransfer?.setData("text/plain", n.path);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "copy";
  }

  // Wurzel und ihre Kinder sind anfangs aufgeklappt.
  $effect(() => {
    for (const r of roots()) open[r.path] ??= true;
  });
</script>

{#snippet item(n: DataNode, depth: number)}
  {@const q = query.trim().toLowerCase()}
  {#if !q || matches(n, q)}
    {@const children = kids(n)}
    {@const expanded = q ? true : !!open[n.path]}
    <div class="row" style="padding-left: {depth * 12 + 4}px" draggable="true" role="treeitem" tabindex="-1"
      aria-selected="false" aria-expanded={children.length ? expanded : undefined} data-path={n.path}
      title={`${n.name}: ${t(`data.type.${typeKey(n)}`)} ${cardinality(n)}${n.doc ? `\n${n.doc}` : ""}`}
      ondragstart={(e) => dragstart(e, n)}>
      {#if children.length}
        <button class="twisty" aria-label={expanded ? t("tree.collapse") : t("tree.expand")} onclick={() => (open[n.path] = !open[n.path])}>{expanded ? "▾" : "▸"}</button>
      {:else}<span class="twisty"></span>{/if}
      <span class="icon" class:attr={n.attr}>{icons[typeKey(n)]}</span>
      <span class="name">{n.name}</span>
      <span class="card" class:rep={n.max === -1 || n.max > 1} class:opt={n.min === 0}>{cardinality(n)}</span>
    </div>
    {#if expanded}
      {#each children as c (c.path)}{@render item(c, depth + 1)}{/each}
    {/if}
  {/if}
{/snippet}

<div class="data" data-testid="data">
  <div class="tools">
    <input type="search" placeholder={t("data.search")} aria-label={t("data.search")} bind:value={query} />
    <button onclick={() => loadSchema(askPath)}>{t("data.load")}</button>
  </div>
  {#if data.schema}
    <p class="hint">{t("data.dragHint")}</p>
    <div role="tree" aria-label={t("panel.data")}>
      {#each roots() as r (r.path)}{@render item(r, 0)}{/each}
    </div>
  {:else}
    <p class="hint">{t("data.none")}</p>
  {/if}
</div>

<style>
  .data {
    padding: var(--space-2) 0;
  }
  .tools {
    display: flex;
    gap: var(--space-1);
    padding: 0 var(--space-2) var(--space-2);
  }
  .tools input {
    flex: 1;
    min-width: 0;
  }
  .hint {
    color: var(--text-muted);
    font-size: 12px;
    padding: 0 var(--space-3);
    margin: 0 0 var(--space-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin: 0 var(--space-1);
    border-radius: 4px;
    cursor: grab;
    white-space: nowrap;
  }
  .row:hover {
    background: var(--surface-2);
  }
  .twisty {
    width: 16px;
    border: 0;
    background: none;
    padding: 0;
    color: var(--text-muted);
  }
  .icon {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--accent);
    min-width: 26px;
    text-align: center;
  }
  .icon.attr {
    color: var(--warning);
  }
  .name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .card {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-muted);
    padding-right: var(--space-2);
  }
  .card.rep {
    color: var(--accent);
  }
</style>
