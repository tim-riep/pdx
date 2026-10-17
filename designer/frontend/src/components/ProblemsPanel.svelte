<script lang="ts">
  import { label, t } from "../lib/i18n";
  import { node } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";
  import { ui } from "../lib/project.svelte";
  import { entries, type Entry } from "../lib/problems.svelte";

  const list = $derived(entries());
  const errors = $derived(list.filter((e) => e.level === "Fehler").length);

  // Klick führt zum Element: Auswahl, Hierarchie und Canvas folgen.
  function go(e: Entry) {
    if (!e.ref) return;
    if (!node(e.ref)) {
      ui.notice = t("problems.missing");
      return;
    }
    select(e.ref, null, true);
  }

  const where = (e: Entry) => {
    if (e.file) return e.file.replace(/^samples\//, "");
    const n = node(e.ref);
    if (n) return label(n.type, n.name);
    return e.ref ? e.ref : t("problems.document");
  };
</script>

<div class="problems" data-testid="problems">
  {#if list.length === 0}
    <p class="none">✓ {t("problems.none")}</p>
  {:else}
    <p class="summary">
      {t("problems.errors", { n: errors })}, {t("problems.hints", { n: list.length - errors })}
    </p>
    <ul>
      {#each list as e, i (i)}
        <li class:error={e.level === "Fehler"} class:selected={!!e.ref && selection.ref === e.ref}>
          <button disabled={!e.ref} title={e.ref ? t("problems.goto") : ""} onclick={() => go(e)} data-ref={e.ref}>
            <span class="mark" aria-label={e.level}>{e.level === "Fehler" ? "✗" : "!"}</span>
            <span class="source">{t(`problems.source.${e.source}`)}</span>
            <span class="where">{where(e)}{#if e.ref && node(e.ref)}<code>{e.ref}</code>{/if}</span>
            <span class="msg">{e.msg}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .problems {
    padding: var(--space-1) var(--space-2);
  }
  .none,
  .summary {
    margin: var(--space-1) var(--space-1);
    color: var(--text-muted);
  }
  .none {
    color: var(--ok);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  button {
    display: grid;
    grid-template-columns: 16px 96px minmax(120px, 220px) 1fr;
    gap: var(--space-2);
    align-items: baseline;
    width: 100%;
    text-align: left;
    border: 0;
    background: none;
    border-radius: 3px;
    padding: 2px var(--space-1);
    color: var(--text);
  }
  button:not(:disabled):hover,
  .selected button {
    background: var(--accent-soft);
  }
  button:disabled {
    cursor: default;
    opacity: 1;
  }
  .mark {
    color: var(--warning);
    font-weight: 600;
  }
  .error .mark {
    color: var(--danger);
  }
  .source {
    color: var(--text-muted);
  }
  .where {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  code {
    margin-left: 6px;
    font-size: 11px;
    color: var(--text-muted);
  }
</style>
