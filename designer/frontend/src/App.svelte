<script lang="ts">
  import { AppAPI } from "./lib/api";
  import { t } from "./lib/i18n";
  import { newProject, openProject, project, save, saveAs, ui } from "./lib/project.svelte";
  import Modal from "./components/Modal.svelte";
  import StartScreen from "./components/StartScreen.svelte";

  type View = "design" | "preview" | "pdf";
  let view: View = $state("design");
  let version = $state("");
  let theme: "light" | "dark" | null = $state(null);
  let pathValue = $state("");

  AppAPI.Info().then((i) => (version = t("status.version", { version: i.version, engine: i.engine })));

  const title = $derived(project.open ? project.name || t("project.untitled") : "");
  $effect(() => {
    document.title = title ? `${title}${project.dirty ? ` (${t("project.dirty")})` : ""} · ${t("app.title")}` : t("app.title");
  });

  function toggleTheme() {
    const dark = theme ? theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    theme = dark ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
  }

  function keydown(e: KeyboardEvent) {
    if (!(e.metaKey || e.ctrlKey) || ui.pathPrompt || ui.closeRequest) return;
    const k = e.key.toLowerCase();
    const act: Record<string, () => unknown> = {
      s: () => (e.shiftKey ? saveAs() : save()),
      o: () => openProject(false),
      n: () => newProject(),
    };
    if (act[k] && (k === "s" ? project.open : true)) {
      e.preventDefault();
      act[k]();
    }
  }

  function answerPath(value: string | null) {
    ui.pathPrompt?.resolve(value && value.trim() ? value.trim() : null);
    ui.pathPrompt = null;
    pathValue = "";
  }

  async function closeAnswer(choice: "save" | "discard" | "cancel") {
    ui.closeRequest = false;
    if (choice === "cancel") return;
    if (choice === "save" && !(await save())) return;
    AppAPI.ConfirmClose();
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="shell">
  <header class="toolbar">
    <strong>{t("app.title")}</strong>
    <div class="group">
      <button onclick={() => newProject()}>{t("action.new")}</button>
      <button onclick={() => openProject(false)}>{t("action.open")}</button>
      <button disabled={!project.open} onclick={() => save()}>{t("action.save")}</button>
    </div>
    {#if project.open}
      <span class="doc" data-testid="doc-title">{title}{#if project.dirty}<em> ({t("project.dirty")})</em>{/if}</span>
    {/if}
    <div class="views" role="tablist">
      {#each [["design", "view.design"], ["preview", "view.preview"], ["pdf", "view.pdf"]] as [v, key]}
        <button role="tab" aria-selected={view === v} class:active={view === v} onclick={() => (view = v as View)}>
          {t(key as "view.design")}
        </button>
      {/each}
    </div>
    <button class="icon" title={t("theme.toggle")} aria-label={t("theme.toggle")} onclick={toggleTheme}>◐</button>
  </header>
  <aside class="left"><h2>{t("panel.data")}</h2><h2>{t("panel.hierarchy")}</h2></aside>
  <main class="canvas" data-testid="canvas">
    {#if project.open}
      <p>{t("canvas.empty")}</p>
    {:else}
      <StartScreen />
    {/if}
  </main>
  <aside class="right"><h2>{t("panel.inspector")}</h2></aside>
  <section class="bottom"><h2>{t("panel.problems")}</h2></section>
  <footer class="status" data-testid="status">
    {#if ui.error}<span class="error" role="alert">{ui.error}</span>{:else if ui.notice}<span>{ui.notice}</span>{/if}
    <span class="version">{version}</span>
  </footer>
</div>

{#if ui.pathPrompt}
  <Modal title={t("path.title")}>
    <label>
      {ui.pathPrompt.title}
      <!-- svelte-ignore a11y_autofocus -->
      <input autofocus style="width: 100%" bind:value={pathValue} onkeydown={(e) => e.key === "Enter" && answerPath(pathValue)} />
    </label>
    {#snippet actions()}
      <button onclick={() => answerPath(null)}>{t("action.cancel")}</button>
      <button class="primary" onclick={() => answerPath(pathValue)}>{t("action.ok")}</button>
    {/snippet}
  </Modal>
{/if}

{#if ui.closeRequest}
  <Modal title={t("close.title")}>
    <p>{t("close.text", { name: title })}</p>
    {#snippet actions()}
      <button onclick={() => closeAnswer("cancel")}>{t("action.cancel")}</button>
      <button onclick={() => closeAnswer("discard")}>{t("action.discard")}</button>
      <button class="primary" onclick={() => closeAnswer("save")}>{t("action.save")}</button>
    {/snippet}
  </Modal>
{/if}

<style>
  .shell {
    display: grid;
    height: 100%;
    grid-template-columns: 260px 1fr 300px;
    grid-template-rows: auto 1fr 140px auto;
    grid-template-areas:
      "toolbar toolbar toolbar"
      "left canvas right"
      "left bottom right"
      "status status status";
  }
  .toolbar {
    grid-area: toolbar;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }
  .group {
    display: flex;
    gap: var(--space-1);
  }
  .doc em {
    color: var(--text-muted);
    font-style: normal;
  }
  .views {
    display: flex;
    gap: 2px;
    margin-left: auto;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 2px;
  }
  .views button {
    border: 0;
    background: none;
  }
  .views button.active {
    background: var(--surface);
    box-shadow: var(--shadow);
  }
  .icon {
    border: 0;
    background: none;
  }
  .left,
  .right,
  .bottom {
    background: var(--surface);
    padding: var(--space-3);
    overflow: auto;
  }
  .left {
    grid-area: left;
    border-right: 1px solid var(--border);
  }
  .right {
    grid-area: right;
    border-left: 1px solid var(--border);
  }
  .bottom {
    grid-area: bottom;
    border-top: 1px solid var(--border);
  }
  h2 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: 0 0 var(--space-2);
  }
  .canvas {
    grid-area: canvas;
    background: var(--canvas);
    display: grid;
    place-items: center;
    color: var(--text-muted);
    overflow: auto;
  }
  .status {
    grid-area: status;
    display: flex;
    gap: var(--space-3);
    padding: var(--space-1) var(--space-3);
    background: var(--surface);
    border-top: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 12px;
  }
  .status .version {
    margin-left: auto;
  }
  .error {
    color: var(--danger);
  }
</style>
