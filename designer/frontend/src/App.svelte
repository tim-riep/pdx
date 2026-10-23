<script lang="ts">
  import { AppAPI, SampleAPI } from "./lib/api";
  import CommandPalette from "./components/CommandPalette.svelte";
  import AboutDialog from "./components/AboutDialog.svelte";
  import { closePalette, openPalette, palette, type PaletteItem } from "./lib/palette.svelte";
  import { checkConformance } from "./lib/conformance.svelte";
  import { insertAtSelection, kinds } from "./lib/insert";
  import { loadSchema } from "./lib/data.svelte";
  import { label } from "./lib/i18n";
  import { askPath } from "./lib/project.svelte";
  import { apply, isPositioned, node } from "./lib/template.svelte";
  import { toMM } from "./lib/geometry";
  import { fmtLen, step as stepLen } from "./lib/units.svelte";
  import { t } from "./lib/i18n";
  import { newProject, openProject, project, save, saveAs, ui } from "./lib/project.svelte";
  import Modal from "./components/Modal.svelte";
  import StartScreen from "./components/StartScreen.svelte";
  import Canvas from "./components/canvas/Canvas.svelte";
  import Tabs from "./components/Tabs.svelte";
  import XPathPanel from "./components/XPathPanel.svelte";
  import ProblemsPanel from "./components/ProblemsPanel.svelte";
  import ContextMenu from "./components/ContextMenu.svelte";
  import PdfView from "./components/PdfView.svelte";
  import SamplesPanel from "./components/SamplesPanel.svelte";
  import SchemaDiffDialog from "./components/SchemaDiffDialog.svelte";
  import PublishDialog from "./components/PublishDialog.svelte";
  import LicensesDialog from "./components/LicensesDialog.svelte";
  import { sd } from "./lib/schemadiff.svelte";
  import { closeMenu, duplicate, menu, moveBy, remove } from "./lib/menu.svelte";
  import StylesPanel from "./components/StylesPanel.svelte";
  import ConformancePanel from "./components/ConformancePanel.svelte";
  import AccessibilityPanel from "./components/AccessibilityPanel.svelte";
  import { access, refreshAccess } from "./lib/access.svelte";
  import { entries } from "./lib/problems.svelte";
  import Hierarchy from "./components/Hierarchy.svelte";
  import Inspector from "./components/Inspector.svelte";
  import DataPanel from "./components/DataPanel.svelte";
  import { clearSelection, select, selection } from "./lib/selection.svelte";
  import { redo, tpl, undo } from "./lib/template.svelte";
  import { render, setView, type View } from "./lib/render.svelte";
  let version = $state("");
  let theme: "light" | "dark" | null = $state(null);
  let pathValue = $state("");
  let leftTab = $state("hierarchy");
  let rightTab = $state("inspector");
  let bottomTab = $state("problems");

  // The accessibility analysis runs again after every change, as long as
  // its panel is open or the reading order is shown.
  $effect(() => {
    const rev = project.rev;
    if (!project.open || (bottomTab !== "access" && !access.showOrder && !access.showTabs)) return;
    const timer = setTimeout(refreshAccess, rev ? 300 : 0);
    return () => clearTimeout(timer);
  });
  let about = $state(false);
  let publish = $state(false);
  let licenses = $state(false);
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

  const mac = typeof navigator !== "undefined" && /Mac/.test(navigator.platform);
  const mod = mac ? "⌘" : "Strg+";

  // Entries of the command palette. Elements with selection only if there is one.
  function paletteItems(): PaletteItem[] {
    const g = (k: string) => t(`palette.group.${k}`);
    const items: PaletteItem[] = [
      { id: "new", label: t("action.new"), group: g("file"), shortcut: `${mod}N`, run: () => newProject() },
      { id: "open", label: t("action.open"), group: g("file"), shortcut: `${mod}O`, run: () => openProject(false) },
    ];
    if (project.open) {
      items.push(
        { id: "save", label: t("action.save"), group: g("file"), shortcut: `${mod}S`, run: () => save() },
        { id: "publish", label: t("publish.open"), group: g("file"), run: () => (publish = true) },
        { id: "saveAs", label: t("action.saveAs"), group: g("file"), shortcut: `${mod}⇧S`, run: () => saveAs() },
        { id: "undo", label: t("action.undo"), group: g("edit"), shortcut: `${mod}Z`, run: () => undo() },
        { id: "redo", label: t("action.redo"), group: g("edit"), shortcut: `${mod}⇧Z`, run: () => redo() },
      );
      if (selection.ref) {
        const r = selection.ref;
        items.push(
          { id: "dup", label: t("menu.duplicate"), group: g("edit"), shortcut: `${mod}D`, run: () => duplicate(r) },
          { id: "up", label: t("menu.up"), group: g("edit"), run: () => moveBy(r, -1) },
          { id: "down", label: t("menu.down"), group: g("edit"), run: () => moveBy(r, 1) },
          { id: "del", label: t("menu.remove"), group: g("edit"), run: () => remove(r) },
        );
      }
      for (const [v, key] of [["design", "view.design"], ["preview", "view.preview"], ["pdf", "view.pdf"]]) {
        items.push({ id: `view-${v}`, label: t("palette.view", { view: t(key) }), group: g("view"), run: () => setView(v as View) });
      }
      for (const k of kinds) {
        items.push({ id: `insert-${k}`, label: t("palette.insert", { el: t(`insert.${k}`) }), group: g("insert"), run: () => insertAtSelection(k) });
      }
      const panels: [string, string, () => void][] = [
        ["data", "panel.data", () => (leftTab = "data")],
        ["hierarchy", "panel.hierarchy", () => (leftTab = "hierarchy")],
        ["samples", "panel.samples", () => (leftTab = "samples")],
        ["inspector", "panel.inspector", () => (rightTab = "inspector")],
        ["styles", "panel.styles", () => (rightTab = "styles")],
        ["problems", "panel.problems", () => (bottomTab = "problems")],
        ["conformance", "panel.conformance", () => (bottomTab = "conformance")],
        ["access", "panel.access", () => (bottomTab = "access")],
        ["xpath", "panel.xpath", () => (bottomTab = "xpath")],
      ];
      for (const [id, key, run] of panels) items.push({ id: `panel-${id}`, label: t("palette.panel", { panel: t(key) }), group: g("panel"), run });
      items.push(
        { id: "conf", label: t("conformance.check"), group: g("check"), run: () => ((bottomTab = "conformance"), checkConformance()) },
        { id: "xsd", label: t("data.load"), group: g("data"), run: () => loadSchema(askPath) },
        ...(["minimal", "maximal", "lang"] as const).map((k) => ({
          id: `edge-${k}`, label: `${t("samples.edge")}: ${t(`samples.edge.${k}`)}`, group: g("data"),
          run: () => SampleAPI.Generate(k).then(() => (leftTab = "samples")).catch((e) => (ui.error = String(e?.message ?? e))),
        })),
      );
      // All elements with ID: go to.
      for (const n of Object.values(tpl.snapshot?.nodes ?? {})) {
        if (!n || n.ref.startsWith("~")) continue;
        items.push({ id: `goto-${n.ref}`, label: label(n.type, n.name), hint: n.ref, group: g("goto"), run: () => { select(n.ref, null, true); leftTab = "hierarchy"; } });
      }
    }
    items.push({ id: "theme", label: t("theme.toggle"), group: g("view"), run: toggleTheme });
    items.push({ id: "about", label: t("about.open"), group: g("view"), run: () => (about = true) });
    items.push({ id: "licenses", label: t("licenses.open"), group: g("view"), run: () => (licenses = true) });
    return items;
  }

  // nudge moves a positioned element by dx, dy millimeters.
  function nudge(dx: number, dy: number) {
    const n = node(selection.ref);
    if (!n || !isPositioned(n)) return false;
    const x = toMM(n.attrs.x ?? "0mm"), y = toMM(n.attrs.y ?? "0mm");
    if (x === null || y === null) return false; // for example a token
    const cmds = [];
    if (dx) cmds.push({ op: "setAttr", ref: n.ref, name: "x", value: fmtLen(Math.max(0, x + dx)) });
    if (dy) cmds.push({ op: "setAttr", ref: n.ref, name: "y", value: fmtLen(Math.max(0, y + dy)) });
    apply(t("cmd.nudge"), { op: "batch", cmds: cmds as never });
    return true;
  }

  // In input fields their own keys apply (text undo, delete).
  const editing = (e: KeyboardEvent) =>
    e.target instanceof HTMLElement && (e.target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName));


  function keydown(e: KeyboardEvent) {
    if (ui.pathPrompt || ui.closeRequest) return;
    if ((e.metaKey || e.ctrlKey) && (e.key.toLowerCase() === "k" || (e.shiftKey && e.key.toLowerCase() === "p"))) {
      e.preventDefault();
      if (palette.open) closePalette();
      else openPalette();
      return;
    }
    if (palette.open) return;
    // Arrows move positioned elements by 1 mm (or 1 pt), with
    // Shift by 10.
    const arrows: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
    if (arrows[e.key] && !e.altKey && !e.metaKey && !e.ctrlKey && !editing(e) && selection.ref && render.view === "design") {
      const step = stepLen(e.shiftKey ? 10 : 1);
      if (nudge(arrows[e.key][0] * step, arrows[e.key][1] * step)) {
        e.preventDefault();
        return;
      }
    }
    if (e.key === "Escape") {
      if (menu.open) {
        closeMenu();
        return;
      }
      clearSelection();
    }
    if ((e.key === "Delete" || e.key === "Backspace") && !editing(e) && selection.ref) {
      e.preventDefault();
      remove(selection.ref);
      return;
    }
    // Alt with arrow moves among the siblings.
    if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown") && !editing(e) && selection.ref) {
      e.preventDefault();
      moveBy(selection.ref, e.key === "ArrowUp" ? -1 : 1);
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
      d: () => selection.ref && duplicate(selection.ref),
    };
    if (!act[k] || (k === "z" || k === "y" || k === "d") && editing(e)) return;
    if ((k === "s" || k === "z" || k === "y" || k === "d") && !project.open) return;
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
      <button disabled={!project.open} data-testid="publish-open" onclick={() => (publish = true)}>{t("publish.open")}</button>
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
    <button data-testid="licenses-open" onclick={() => (licenses = true)}>{t("licenses.open")}</button>
    <button class="icon" title={t("theme.toggle")} aria-label={t("theme.toggle")} onclick={toggleTheme}>◐</button>
  </header>
  <aside class="left">
    <Tabs tabs={[["data", t("panel.data")], ["hierarchy", t("panel.hierarchy")], ["samples", t("panel.samples")]]} bind:active={leftTab}>
      {#if leftTab === "hierarchy"}<Hierarchy />{:else if leftTab === "samples"}<SamplesPanel />{:else}<DataPanel />{/if}
    </Tabs>
  </aside>
  <main class="canvas" data-testid="canvas">
    {#if project.open && render.view === "pdf"}
      <PdfView />
    {:else if project.open}
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
    <Tabs tabs={[["problems", problemCount ? t("problems.count", { n: problemCount }) : t("panel.problems")], ["conformance", t("panel.conformance")], ["access", t("panel.access")], ["xpath", t("panel.xpath")]]} bind:active={bottomTab}>
      {#if bottomTab === "xpath"}
        <XPathPanel />
      {:else if bottomTab === "problems"}
        <ProblemsPanel />
      {:else if bottomTab === "access"}
        <AccessibilityPanel />
      {:else}
        <ConformancePanel />
      {/if}
    </Tabs>
  </section>
  <footer class="status" data-testid="status">
    {#if ui.error}<span class="error" role="alert">{ui.error}</span>{:else if ui.notice}<span>{ui.notice}</span>{/if}
    <button class="version" onclick={() => (about = true)} title={t("about.open")}>{version}</button>
  </footer>
</div>

<ContextMenu />
{#if palette.open}<CommandPalette items={paletteItems} />{/if}
{#if about}<AboutDialog {version} onclose={() => (about = false)} onlicenses={() => ((about = false), (licenses = true))} />{/if}
{#if licenses}<LicensesDialog onclose={() => (licenses = false)} />{/if}
{#if sd.open}<SchemaDiffDialog />{/if}
{#if publish}<PublishDialog onclose={() => (publish = false)} />{/if}

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
    border: 0;
    background: none;
    padding: 0;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }
  .status .version:hover {
    text-decoration: underline;
  }
  .error {
    color: var(--danger);
  }
</style>
