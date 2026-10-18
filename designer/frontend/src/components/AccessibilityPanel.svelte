<script lang="ts">
  import type { StructNode } from "../lib/api";
  import { has, label, profileShort, t } from "../lib/i18n";
  import { node } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";
  import { project, ui } from "../lib/project.svelte";
  import { access, refreshAccess } from "../lib/access.svelte";
  import MatterhornChecklist from "./MatterhornChecklist.svelte";

  const a = $derived(access.data);

  function go(ref: string) {
    if (!ref) return;
    if (!node(ref)) {
      ui.notice = t("problems.missing");
      return;
    }
    select(ref, null, true);
  }

  const typeLabel = (type: string) => (has(`struct.${type}`) ? t(`struct.${type}`) : type);
  const where = (id: string) => {
    const n = node(id);
    return n ? label(n.type, n.name) : t("problems.document");
  };
</script>

{#snippet tree(n: StructNode, depth: number)}
  <li>
    <button class="row" class:sel={!!n.id && selection.ref === n.id} disabled={!n.id} style:padding-left="{depth * 14 + 4}px"
      onclick={() => go(n.id)} data-struct={n.type} data-ref={n.id}>
      <span class="type">{typeLabel(n.type)}</span>
      {#if n.id && node(n.id)}<code>{n.id}</code>{/if}
      {#if n.alt}<span class="alt" title={t("prop.alt")}>„{n.alt}“</span>{:else if n.text}<span class="text">{n.text}</span>{/if}
      {#if n.lang}<span class="lang">{n.lang}</span>{/if}
    </button>
    {#if n.kids.length}
      <ul>{#each n.kids as k, i (i)}{#if k}{@render tree(k, depth + 1)}{/if}{/each}</ul>
    {/if}
  </li>
{/snippet}

<div class="access" data-testid="access">
  <div class="bar">
    <button disabled={!project.open || access.loading} onclick={refreshAccess}>{t("access.refresh")}</button>
    <label class="toggle"><input type="checkbox" bind:checked={access.showOrder} data-testid="access-order" /> {t("access.order")}</label>
    {#if access.loading}<span class="spin" aria-hidden="true"></span>{/if}
    {#if a}
      <span class="muted">{a.assumed ? t("access.assumed", { profile: profileShort(a.profile) }) : t("access.profile", { profile: profileShort(a.profile) })}</span>
    {/if}
  </div>
  {#if access.error}<p class="verdict" role="alert">✗ {access.error}</p>{/if}

  {#if a}
    {#each a.problems as p, i (i)}
      <button class="item" disabled={!p.id} onclick={() => go(p.id)}>
        <span class="where">{p.id ? where(p.id) : t("problems.document")}</span>
        <span>{p.msg}</span>
      </button>
    {/each}
    {#if a.tree}
      <p class="verdict" class:ok={a.findings.length === 0} role="status" data-testid="access-verdict">
        <span aria-hidden="true">{a.findings.length === 0 ? "✓" : "✗"}</span>
        {a.findings.length === 0 ? t("access.ok") : t("access.bad", { n: a.findings.length })}
      </p>
      {#each a.findings as f, i (i)}
        <button class="item" disabled={!f.id} onclick={() => go(f.id)} data-ref={f.id}>
          <span class="where">{f.id ? where(f.id) : t("problems.document")}{#if f.id && node(f.id)}<code>{f.id}</code>{/if}</span>
          <span>{f.msg}</span>
        </button>
      {/each}
      <p class="hint">{t("access.hint")}</p>
      <h3>{t("access.tree")}</h3>
      <ul class="tree" data-testid="access-tree">{@render tree(a.tree, 0)}</ul>
    {/if}
    <MatterhornChecklist />
  {:else if !access.loading}
    <p class="muted">{t("access.empty")}</p>
  {/if}
</div>

<style>
  .access {
    padding: var(--space-2) var(--space-3);
  }
  .bar {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .toggle {
    display: flex;
    gap: 4px;
    align-items: center;
    white-space: nowrap;
  }
  .muted,
  .hint {
    color: var(--text-muted);
  }
  .hint {
    font-size: 11px;
    margin: var(--space-1) 0;
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  .verdict {
    margin: var(--space-2) 0 var(--space-1);
    color: var(--danger);
    font-weight: 600;
  }
  .verdict.ok {
    color: var(--ok);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  button.item,
  button.row {
    display: grid;
    width: 100%;
    text-align: left;
    border: 0;
    background: none;
    border-radius: 3px;
    padding: 2px var(--space-1);
    color: var(--text);
    gap: var(--space-2);
    align-items: baseline;
  }
  button.item {
    grid-template-columns: minmax(120px, 220px) 1fr;
  }
  button.row {
    display: flex;
    overflow: hidden;
    white-space: nowrap;
  }
  button.item:not(:disabled):hover,
  button.row:not(:disabled):hover {
    background: var(--accent-soft);
  }
  button.row.sel {
    background: var(--accent-soft);
  }
  button:disabled {
    opacity: 1;
    cursor: default;
  }
  .type {
    font-weight: 600;
  }
  code,
  .lang {
    color: var(--text-muted);
    font-size: 11px;
  }
  .text,
  .alt {
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .alt {
    font-style: italic;
  }
  .where {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .spin {
    display: inline-block;
    width: 10px;
    height: 10px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
