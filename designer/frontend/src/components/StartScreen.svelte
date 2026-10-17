<script lang="ts">
  import { ProjectAPI, type RecentFile, type Recovery } from "../lib/api";
  import { t } from "../lib/i18n";
  import { newProject, openPath, openProject, recover } from "../lib/project.svelte";

  let recent: RecentFile[] = $state([]);
  let recoveries: Recovery[] = $state([]);

  ProjectAPI.Recent().then((r) => (recent = r));
  ProjectAPI.Recoverable().then((r) => (recoveries = r));

  async function discard(id: string) {
    await ProjectAPI.Discard(id);
    recoveries = recoveries.filter((r) => r.id !== id);
  }
  const when = (s: unknown) => new Date(String(s)).toLocaleString("de-DE");
</script>

<div class="start" data-testid="start">
  <h1>{t("start.title")}</h1>
  <div class="buttons">
    <button class="primary" onclick={() => newProject()}>{t("action.new")}</button>
    <button onclick={() => openProject(false)}>{t("action.open")}</button>
    <button onclick={() => openProject(true)}>{t("action.openDir")}</button>
  </div>

  {#if recoveries.length}
    <section class="recovery">
      <h2>{t("start.recovery")}</h2>
      {#each recoveries as r (r.id)}
        <div class="row">
          <span><strong>{r.name || t("project.untitled")}</strong> {t("start.recoveryHint", { time: when(r.saved) })}</span>
          <button onclick={() => recover(r.id)}>{t("action.restore")}</button>
          <button onclick={() => discard(r.id)}>{t("action.discard")}</button>
        </div>
      {/each}
    </section>
  {/if}

  <section>
    <h2>{t("start.recent")}</h2>
    {#each recent as r (r.path)}
      <button class="link" title={r.path} onclick={() => openPath(r.path)}>{r.name}<small>{r.path}</small></button>
    {:else}
      <p class="muted">{t("start.none")}</p>
    {/each}
  </section>
</div>

<style>
  .start {
    width: min(560px, 90%);
    color: var(--text);
  }
  h1 {
    font-size: 20px;
    font-weight: 600;
  }
  h2 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-4) 0 var(--space-2);
  }
  .buttons {
    display: flex;
    gap: var(--space-2);
  }
  .recovery {
    border: 1px solid var(--warning);
    border-radius: var(--radius);
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
  }
  .row {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .row span {
    flex: 1;
  }
  .link {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    width: 100%;
    text-align: left;
  }
  .link small {
    color: var(--text-muted);
  }
  .muted {
    color: var(--text-muted);
  }
</style>
