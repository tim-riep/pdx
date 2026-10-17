<script lang="ts">
  import { Info } from "../bindings/github.com/tim-riep/pdx/designer/internal/app/appservice";
  import { t } from "./lib/i18n";

  type View = "design" | "preview" | "pdf";
  let view: View = $state("design");
  let version = $state("");
  let theme: "light" | "dark" | null = $state(null);

  Info().then((i) => (version = t("status.version", { version: i.version, engine: i.engine })));

  function toggleTheme() {
    const dark = theme ? theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    theme = dark ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
  }
</script>

<div class="shell">
  <header class="toolbar">
    <strong>{t("app.title")}</strong>
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
  <main class="canvas" data-testid="canvas"><p>{t("canvas.empty")}</p></main>
  <aside class="right"><h2>{t("panel.inspector")}</h2></aside>
  <section class="bottom"><h2>{t("panel.problems")}</h2></section>
  <footer class="status" data-testid="status">{version}</footer>
</div>

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
  .views {
    display: flex;
    gap: 2px;
    margin: 0 auto;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 2px;
  }
  button {
    font: inherit;
    color: inherit;
    background: none;
    border: 0;
    border-radius: calc(var(--radius) - 2px);
    padding: var(--space-1) var(--space-3);
    cursor: pointer;
  }
  button.active {
    background: var(--surface);
    box-shadow: var(--shadow);
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
  }
  .status {
    grid-area: status;
    padding: var(--space-1) var(--space-3);
    background: var(--surface);
    border-top: 1px solid var(--border);
    color: var(--text-muted);
    font-size: 12px;
  }
</style>
