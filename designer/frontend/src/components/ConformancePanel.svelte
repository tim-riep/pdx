<script lang="ts">
  import type { ConformanceItem } from "../lib/api";
  import { label, t } from "../lib/i18n";
  import { node } from "../lib/template.svelte";
  import { select } from "../lib/selection.svelte";
  import { project, ui } from "../lib/project.svelte";
  import { cancelConformance, checkConformance, conf } from "../lib/conformance.svelte";

  const st = $derived(conf.state);
  const running = $derived(st?.state === "running");
  const stale = $derived(!!st && st.state !== "running" && st.state !== "idle" && st.rev !== project.rev);
  let open: Record<number, boolean> = $state({});

  function go(ref: string) {
    if (!node(ref)) {
      ui.notice = t("problems.missing");
      return;
    }
    select(ref, null, true);
  }

  const where = (it: ConformanceItem) => {
    const n = node(it.ref);
    if (n) return label(n.type, n.name);
    return it.page ? t("conformance.page", { n: it.page }) : t("problems.document");
  };
  const source = $derived(st?.sample ? t("xpath.withSample", { file: st.sample.replace(/^samples\//, "") }) : t("xpath.withPlaceholder"));
</script>

<div class="conformance" data-testid="conformance">
  <div class="bar">
    {#if running}
      <button onclick={cancelConformance}>{t("conformance.cancel")}</button>
      <span class="muted" role="status"><span class="spin" aria-hidden="true"></span>{t("conformance.running")}</span>
    {:else}
      <button class="primary" disabled={!project.open} onclick={checkConformance}>{st && st.state !== "idle" ? t("conformance.again") : t("conformance.check")}</button>
      {#if !st || st.state === "idle"}<span class="muted">{t("conformance.idle")}</span>{/if}
    {/if}
  </div>

  {#if st && st.state === "done"}
    <p class="verdict" class:ok={st.compliant} role="status" data-testid="conformance-verdict">
      <span aria-hidden="true">{st.compliant ? "✓" : "✗"}</span>
      {st.compliant ? t("conformance.ok") : t("conformance.bad", { n: st.findings.length })}
      <span class="muted" title={st.runner}>{source}, {t("conformance.meta", { runner: st.runner.replace(/ \(.*\)$/, ""), seconds: (st.millis / 1000).toFixed(1) })}</span>
    </p>
  {:else if st && st.state === "error"}
    <p class="verdict" role="alert">✗ {t("conformance.error")}: {st.error}</p>
    {#each st.problems as p, i (i)}
      <button class="item" disabled={!p.id} onclick={() => go(p.id)}>
        <span class="where">{p.id ? where({ ref: p.id, page: 0, context: "", message: "" }) : t("problems.document")}</span>
        <span>{p.msg}</span>
      </button>
    {/each}
  {/if}
  {#if stale}<p class="stale">{t("conformance.stale")}</p>{/if}
  {#each st?.notes ?? [] as n, i (i)}<p class="muted">{n}</p>{/each}

  {#if st && st.state === "done"}
    <ul>
      {#each st.findings as f, i (i)}
        <li>
          <button class="rule" aria-expanded={!!open[i]} onclick={() => (open[i] = !open[i])}>
            <span class="twisty">{open[i] ? "▾" : "▸"}</span>
            <span class="clause">{f.test ? t("conformance.clauseTest", { clause: f.clause, test: f.test }) : t("conformance.clause", { clause: f.clause })}</span>
            <span class="desc">{f.description}</span>
            <span class="count">{f.count}</span>
          </button>
          {#if open[i]}
            {#each f.items as it, j (j)}
              <button class="item" disabled={!it.ref} title={it.context} onclick={() => go(it.ref)} data-ref={it.ref}>
                <span class="where">{where(it)}{#if it.ref && node(it.ref)}<code>{it.ref}</code>{/if}</span>
                <span>{it.message}</span>
              </button>
            {/each}
            {#if f.count > f.items.length}<p class="muted more">{t("conformance.more", { n: f.count - f.items.length })}</p>{/if}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .conformance {
    padding: var(--space-2) var(--space-3);
  }
  .bar {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .muted {
    color: var(--text-muted);
  }
  .verdict {
    margin: var(--space-2) 0 var(--space-1);
    color: var(--danger);
    font-weight: 600;
  }
  .verdict.ok {
    color: var(--ok);
  }
  .verdict .muted {
    font-weight: normal;
    margin-left: var(--space-2);
  }
  .stale {
    color: var(--warning);
    margin: var(--space-1) 0;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  button.rule,
  button.item {
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
  button.rule {
    grid-template-columns: 12px 170px 1fr auto;
  }
  button.item {
    grid-template-columns: minmax(120px, 220px) 1fr;
    padding-left: 24px;
  }
  button.rule:hover,
  button.item:not(:disabled):hover {
    background: var(--accent-soft);
  }
  button.item:disabled {
    opacity: 1;
    cursor: default;
  }
  .clause,
  .count,
  code {
    color: var(--text-muted);
  }
  code {
    margin-left: 6px;
    font-size: 11px;
  }
  .where {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .more {
    padding-left: 24px;
    margin: 2px 0;
  }
  .spin {
    display: inline-block;
    width: 10px;
    height: 10px;
    margin-right: 6px;
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
