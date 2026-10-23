<script lang="ts">
  import { SampleAPI, message, type Result, type Samples } from "../lib/api";
  import { t } from "../lib/i18n";
  import { askPath, project, ui } from "../lib/project.svelte";
  import { tpl } from "../lib/template.svelte";
  import { problems } from "../lib/problems.svelte";

  let samples: Samples | null = $state(null);

  async function load() {
    try {
      samples = await SampleAPI.List();
    } catch {
      samples = null;
    }
  }

  // Reload after every change to the document and after selection.
  $effect(() => {
    void project.rev;
    void project.doc;
    load();
  });

  async function run(f: () => Promise<Result | void>) {
    try {
      const res = await f();
      if (res) tpl.history = res.history;
      ui.error = "";
    } catch (e) {
      ui.error = message(e);
    }
    await load();
  }

  async function importFile() {
    try {
      const res = await SampleAPI.ImportDialog();
      if (res) tpl.history = res.history;
      ui.error = "";
    } catch (e) {
      if (!message(e).includes("nicht verfügbar")) {
        ui.error = message(e);
        return;
      }
      const path = await askPath(t("samples.path"));
      if (!path) return;
      await run(() => SampleAPI.Import(path));
      return;
    }
    await load();
  }

  // Findings of the check per file (sample data against the data XSD).
  const findings = $derived.by(() => {
    const out: Record<string, number> = {};
    for (const p of problems.report?.problems ?? []) if (p.file) out[p.file] = (out[p.file] ?? 0) + 1;
    return out;
  });

  const edges = ["minimal", "maximal", "lang"] as const;
</script>

<div class="samples" data-testid="samples">
  <h3>{t("samples.title")}</h3>
  <p class="muted">{t("samples.hint")}</p>
  {#if samples && samples.items.length === 0}<p class="muted">{t("samples.none")}</p>{/if}
  <ul role="radiogroup" aria-label={t("samples.title")}>
    {#each samples?.items ?? [] as s (s.path)}
      <li class:selected={s.selected} data-sample={s.path}>
        <label>
          <input type="radio" name="sample" checked={s.selected} onchange={() => run(() => SampleAPI.Select(s.path))} />
          <span class="name">{s.name}</span>
        </label>
        {#if findings[s.path]}<span class="bad" title={t("samples.problems", { n: findings[s.path] })}>✗ {findings[s.path]}</span>{/if}
        <span class="size">{t("samples.size", { kb: Math.max(1, Math.round(s.size / 1024)) })}</span>
        <button class="remove" title={t("samples.remove")} aria-label={t("samples.remove")} onclick={() => run(() => SampleAPI.Remove(s.path))}>×</button>
      </li>
    {/each}
  </ul>
  <div class="actions">
    <button onclick={importFile}>{t("samples.import")}</button>
  </div>
  <h3>{t("samples.edge")}</h3>
  <div class="edges">
    {#each edges as k (k)}
      <button title={t(`samples.edge.${k}Hint`)} onclick={() => run(() => SampleAPI.Generate(k))}>{t(`samples.edge.${k}`)}</button>
    {/each}
  </div>
</div>

<style>
  .samples {
    padding: var(--space-2) var(--space-3);
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  h3:first-child {
    margin-top: 0;
  }
  .muted {
    color: var(--text-muted);
    margin: 0 0 var(--space-2);
    font-size: 12px;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 2px 4px;
    border-radius: 4px;
  }
  li.selected {
    background: var(--accent-soft);
  }
  label {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    min-width: 0;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .size {
    color: var(--text-muted);
    font-size: 11px;
  }
  .bad {
    color: var(--danger);
    font-size: 11px;
  }
  .remove {
    border: 0;
    background: none;
    padding: 0 4px;
  }
  .actions,
  .edges {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
    margin-top: var(--space-2);
  }
  .actions button,
  .edges button {
    font-size: 12px;
    padding: 2px 8px;
  }
</style>
