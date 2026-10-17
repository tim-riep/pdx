<script lang="ts">
  import { label, t } from "../lib/i18n";
  import { node } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";
  import { ui } from "../lib/project.svelte";
  import { entries, problems, type Entry } from "../lib/problems.svelte";
  import { sd, type Task } from "../lib/schemadiff.svelte";

  // Eine Aufgabe ist offen, solange eines ihrer Elemente einen Befund hat
  // (oder bei Beispieldaten eine Datei). Abhaken geht auch von Hand.
  function open(task: Task): boolean {
    if (sd.done[task.id]) return false;
    const ps = problems.report?.problems ?? [];
    if (task.samples) return ps.some((p) => p.source === "beispiel");
    return task.refs.some((r) => node(r) && ps.some((p) => p.ref === r));
  }

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
  {#if sd.tasks.length}
    <section class="tasks" data-testid="schema-tasks">
      <h4>
        {t("schemadiff.tasks")}
        <button class="close" title={t("schemadiff.dismiss")} aria-label={t("schemadiff.dismiss")} onclick={() => (sd.tasks = [])}>×</button>
      </h4>
      <ul>
        {#each sd.tasks as task (task.id)}
          {@const isOpen = open(task)}
          <li class:done={!isOpen} data-task={task.id}>
            <input type="checkbox" checked={!isOpen} aria-label={t("schemadiff.done")} onchange={(e) => (sd.done[task.id] = e.currentTarget.checked)} />
            <span class="text">{task.text}</span>
            {#each task.refs.filter((r) => node(r)) as r (r)}
              <button class="ref" onclick={() => go({ level: "", source: "", ref: r, file: "", msg: "" })}>{r}</button>
            {/each}
          </li>
        {/each}
      </ul>
    </section>
  {/if}
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
  .tasks {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: var(--space-1) var(--space-2);
    margin: var(--space-1) 0 var(--space-2);
    background: var(--surface-2);
  }
  .tasks h4 {
    display: flex;
    justify-content: space-between;
    margin: 0 0 4px;
    font-size: 12px;
  }
  .tasks ul li {
    display: flex;
    flex-wrap: wrap;
    gap: 4px var(--space-2);
    align-items: center;
    padding: 1px 0;
  }
  .tasks li.done .text {
    color: var(--text-muted);
    text-decoration: line-through;
  }
  .tasks button.ref,
  .tasks button.close {
    display: inline;
    width: auto;
    padding: 0 4px;
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--accent);
  }
  .tasks button.close {
    color: var(--text-muted);
    font-family: var(--font-ui);
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
