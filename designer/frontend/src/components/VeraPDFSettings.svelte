<script lang="ts">
  import { AppAPI, ConformanceAPI, message, type VeraPDFSetup } from "../lib/api";
  import { t } from "../lib/i18n";
  import { askPath } from "../lib/project.svelte";

  let setup: VeraPDFSetup | null = $state(null);
  let path = $state("");
  let error = $state("");
  let busy = $state(false);

  async function load() {
    busy = true;
    try {
      setup = await ConformanceAPI.VeraPDF();
      path = setup.path;
    } finally {
      busy = false;
    }
  }

  async function save(p: string) {
    busy = true;
    error = "";
    try {
      setup = await ConformanceAPI.SetVeraPDF(p);
      path = setup.path;
    } catch (e) {
      error = message(e);
    } finally {
      busy = false;
    }
  }

  async function pick() {
    let p: string | null = null;
    try {
      p = await AppAPI.PickFile(t("verapdf.pick"), "veraPDF", "*");
    } catch (e) {
      if (!message(e).includes("nicht verfügbar")) {
        error = message(e);
        return;
      }
      p = await askPath(t("verapdf.pick"));
    }
    if (p) await save(p);
  }
</script>

<details class="verapdf" data-testid="verapdf-settings" ontoggle={(e) => (e.currentTarget as HTMLDetailsElement).open && !setup && load()}>
  <summary>veraPDF</summary>
  <div class="body">
    {#if busy && !setup}
      <p class="muted">{t("verapdf.searching")}</p>
    {:else if setup}
      {#if setup.error}
        <p class="bad" role="alert" data-testid="verapdf-status">{setup.error}</p>
      {:else}
        <p data-testid="verapdf-status">{t("verapdf.uses", { runner: setup.runner })}</p>
      {/if}
      {#each setup.notes ?? [] as n, i (i)}<p class="muted">{n}</p>{/each}
      <p class="muted">{t("verapdf.help", { version: setup.version })}</p>
    {/if}
    <label class="row">
      <span>{t("verapdf.path")}</span>
      <input data-testid="verapdf-path" bind:value={path} placeholder={t("verapdf.auto")} spellcheck="false" disabled={busy} />
    </label>
    <div class="actions">
      <button onclick={pick} disabled={busy}>{t("verapdf.choose")}</button>
      <button onclick={() => save(path)} disabled={busy} data-testid="verapdf-apply">{t("verapdf.apply")}</button>
      <button onclick={() => save("")} disabled={busy || !setup?.path} data-testid="verapdf-auto">{t("verapdf.reset")}</button>
    </div>
    {#if error}<p class="bad" role="alert">{error}</p>{/if}
  </div>
</details>

<style>
  .verapdf {
    margin-left: auto;
    font-size: 12px;
  }
  summary {
    cursor: pointer;
    color: var(--text-muted);
  }
  .body {
    margin-top: var(--space-1);
    display: flex;
    flex-direction: column;
    gap: 4px;
    max-width: 520px;
  }
  .body p {
    margin: 0;
  }
  .row {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .row input {
    flex: 1;
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .muted {
    color: var(--text-muted);
  }
  .bad {
    color: var(--danger, #cf222e);
  }
</style>
