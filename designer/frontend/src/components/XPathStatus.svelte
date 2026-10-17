<script lang="ts">
  import { t } from "../lib/i18n";
  import { xp } from "../lib/xpath.svelte";

  let { compact = false }: { compact?: boolean } = $props();

  const info = $derived(xp.info && xp.infoExpr === xp.draft ? xp.info : null);
  const r = $derived(info?.result);
  const bad = $derived(!!r?.error || (info?.problems.length ?? 0) > 0);
  const quote = (v: string) => `„${v.length > 40 ? v.slice(0, 39) + "…" : v}“`;

  const summary = $derived.by(() => {
    if (!info) return "";
    if (!info.context) return t("xpath.noRoot");
    if (r?.error) return r.error;
    if (info.problems.length) return info.problems.map((p) => p.message).join(". ");
    if (!r) return "";
    if (r.noContext) return t("xpath.noContext");
    if (r.kind === "empty") return t("xpath.empty");
    if (r.kind === "boolean") return r.values.map((v) => t(v === "true" ? "xpath.true" : "xpath.false")).join(", ");
    const values = r.values.map(quote).join(", ") + (r.count > r.values.length ? " …" : "");
    if (r.kind === "nodes") return `${t("xpath.count", { n: r.count })}  ${values}`;
    return values;
  });
  const source = $derived(info?.data ? t("xpath.withSample", { file: info.data.replace(/^samples\//, "") }) : t("xpath.withPlaceholder"));
</script>

{#if info}
  <div class="status" class:bad class:compact data-testid="xpath-status" role="status">
    <span class="mark" aria-hidden="true">{bad ? "✗" : "✓"}</span>
    <span class="text" title={summary}>{summary}</span>
    {#if !compact && r && !r.error && !r.noContext}
      <span class="source">{source}{#if r.truth && r.kind !== "boolean"}, {t("xpath.value")} {t(r.truth === "true" ? "xpath.true" : "xpath.false")}{/if}</span>
    {/if}
  </div>
{/if}

<style>
  .status {
    display: flex;
    flex-wrap: wrap;
    gap: 4px var(--space-2);
    align-items: baseline;
    font-size: 12px;
    margin-top: 2px;
    color: var(--text-muted);
  }
  .compact .text {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }
  .compact {
    flex-wrap: nowrap;
  }
  .mark {
    color: var(--ok);
  }
  .bad .mark,
  .bad .text {
    color: var(--danger);
  }
  .source {
    width: 100%;
  }
</style>
