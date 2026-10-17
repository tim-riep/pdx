<script lang="ts">
  import { AppAPI } from "./lib/api";
  import { t } from "./lib/i18n";
  import { newProject, openProject, project, save, saveAs, ui } from "./lib/project.svelte";
  import Modal from "./components/Modal.svelte";
  import StartScreen from "./components/StartScreen.svelte";
  import Canvas from "./components/canvas/Canvas.svelte";
  import Tabs from "./components/Tabs.svelte";
  import XPathPanel from "./components/XPathPanel.svelte";
  import ProblemsPanel from "./components/ProblemsPanel.svelte";
  import StylesPanel from "./components/StylesPanel.svelte";
  import ConformancePanel from "./components/ConformancePanel.svelte";
  import { entries } from "./lib/problems.svelte";
  import Hierarchy from "./components/Hierarchy.svelte";
  import Inspector from "./components/Inspector.svelte";
  import DataPanel from "./components/DataPanel.svelte";
  import { clearSelection, selection } from "./lib/selection.svelte";
  import { apply, node, redo, tpl, undo } from "./lib/template.svelte";
  import { label } from "./lib/i18n";
  import { render, setView, type View } from "./lib/render.svelte";
  let version = $state("");
  let theme: "light" | "dark" | null = $state(null);
  let pathValue = $state("");
  let leftTab = $state("hierarchy");
  let rightTab = $state("inspector");
  let bottomTab = $state("problems");
  const problemCount = $derived(entries().length);

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

  // In Eingabefeldern gelten deren eigene Tasten (Text rückgängig, Löschen).
  const editing = (e: KeyboardEvent) =>
    e.target instanceof HTMLElement && (e.target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName));

  function removeSelection() {
    const n = node(selection.ref);
    if (!n) return;
    apply(t("cmd.remove", { el: label(n.type, n.name) }), { op: "remove", ref: n.ref });
    clearSelection();
  }

  function keydown(e: KeyboardEvent) {
    if (ui.pathPrompt || ui.closeRequest) return;
    if (e.key === "Escape") clearSelection();
    if ((e.key === "Delete" || e.key === "Backspace") && !editing(e) && selection.ref) {
      e.preventDefault();
      removeSelection();
      return;
    }
    if (!(e.metaKey || e.ctrlKey)) return;
    const k = e.key.toLowerCase();
    const act: Record<string, () => unknown> = {
      s: () => (e.shiftKey ? saveAs() : save()),
      o: () => openProject(false),
      n: () => newProject(),
      z: () => (e.shiftKey ? redo() : undo()),
      y: () => redo(),
    };
    if (!act[k] || (k === "z" || k === "y") && editing(e)) return;
    if ((k === "s" || k === "z" || k === "y") && !project.open) return;
    e.preventDefault();
    act[k]();
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
    <div class="group">
      <button disabled={!tpl.history.can[0]} aria-label={t("action.undo")}
        title={tpl.history.undo ? t("action.undoLabel", { label: tpl.history.undo }) : t("action.undo")} onclick={() => undo()}>↶</button>
      <button disabled={!tpl.history.can[1]} aria-label={t("action.redo")}
        title={tpl.history.redo ? t("action.redoLabel", { label: tpl.history.redo }) : t("action.redo")} onclick={() => redo()}>↷</button>
    </div>
    {#if project.open}
      <span class="doc" data-testid="doc-title">{title}{#if project.dirty}<em> ({t("project.dirty")})</em>{/if}</span>
    {/if}
    <div class="views" role="tablist">
      {#each [["design", "view.design"], ["preview", "view.preview"], ["pdf", "view.pdf"]] as [v, key]}
        <button role="tab" aria-selected={render.view === v} class:active={render.view === v} onclick={() => setView(v as View)}>
          {t(key as "view.design")}
        </button>
      {/each}
    </div>
    <button class="icon" title={t("theme.toggle")} aria-label={t("theme.toggle")} onclick={toggleTheme}>◐</button>
  </header>
  <aside class="left">
    <Tabs tabs={[["data", t("panel.data")], ["hierarchy", t("panel.hierarchy")]]} bind:active={leftTab}>
      {#if leftTab === "hierarchy"}<Hierarchy />{:else}<DataPanel />{/if}
    </Tabs>
  </aside>
  <main class="canvas" data-testid="canvas">
    {#if project.open}
      <Canvas />
    {:else}
      <StartScreen />
    {/if}
  </main>
  <aside class="right">
    <Tabs tabs={[["inspector", t("panel.inspector")], ["styles", t("panel.styles")]]} bind:active={rightTab}>
      {#if rightTab === "inspector"}<Inspector />{:else}<StylesPanel onopen={() => (rightTab = "inspector")} />{/if}
    </Tabs>
  </aside>
  <section class="bottom">
    <Tabs tabs={[["problems", problemCount ? t("problems.count", { n: problemCount }) : t("panel.problems")], ["conformance", t("panel.conformance")], ["xpath", t("panel.xpath")]]} bind:active={bottomTab}>
      {#if bottomTab === "xpath"}
        <XPathPanel />
      {:else if bottomTab === "problems"}
        <ProblemsPanel />
      {:else}
        <ConformancePanel />
      {/if}
    </Tabs>
  </section>
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
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
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
  .canvas {
    grid-area: canvas;
    background: var(--canvas);
    display: flex;
    flex-direction: column;
    color: var(--text-muted);
    overflow: hidden;
    min-height: 0;
  }
  .canvas > :global([data-testid="start"]) {
    margin: auto;
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
