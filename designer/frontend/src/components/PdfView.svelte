<script lang="ts">
  // Ansicht PDF: das erzeugte PDF/A-3b mit pdf.js, so wie es ausgeliefert
  // wird. pdf.js und sein Worker liegen im Bündel, nichts kommt aus dem Netz.
  import { getDocument, GlobalWorkerOptions, type PDFDocumentLoadingTask, type PDFDocumentProxy } from "pdfjs-dist";
  import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
  import { t } from "../lib/i18n";
  import { project } from "../lib/project.svelte";

  GlobalWorkerOptions.workerSrc = workerUrl;

  let host: HTMLDivElement | undefined = $state();
  let status = $state<"loading" | "ok" | "error">("loading");
  let error = $state("");
  let pages = $state(0);
  let width = $state(800);
  let seq = 0;
  let doc: PDFDocumentProxy | null = null;
  let task: PDFDocumentLoadingTask | null = null;

  async function load(rev: number) {
    const s = ++seq;
    status = "loading";
    try {
      const res = await fetch(`/pdx/pdf/${rev}`);
      if (!res.ok) throw new Error(await res.text());
      const data = new Uint8Array(await res.arrayBuffer());
      const next = getDocument({ data, enableXfa: false });
      const d = await next.promise;
      if (s !== seq) return void next.destroy();
      task?.destroy();
      task = next;
      doc = d;
      pages = d.numPages;
      await draw();
      status = "ok";
    } catch (e) {
      if (s !== seq) return;
      status = "error";
      error = e instanceof Error ? e.message : String(e);
    }
  }

  // draw zeichnet alle Seiten in Breite des Bereichs, scharf für die
  // Pixeldichte des Bildschirms.
  async function draw() {
    if (!doc || !host) return;
    const canvases: HTMLCanvasElement[] = [];
    const dpr = window.devicePixelRatio || 1;
    for (let i = 1; i <= doc.numPages; i++) {
      const p = await doc.getPage(i);
      const base = p.getViewport({ scale: 1 });
      const scale = Math.min(2.5, (width - 48) / base.width);
      const vp = p.getViewport({ scale: scale * dpr });
      const c = document.createElement("canvas");
      c.width = vp.width;
      c.height = vp.height;
      c.style.width = `${vp.width / dpr}px`;
      c.style.height = `${vp.height / dpr}px`;
      c.dataset.pdfPage = String(i);
      await p.render({ canvas: c, viewport: vp }).promise;
      canvases.push(c);
    }
    host.replaceChildren(...canvases);
  }

  // Neu laden nach Änderungen, kurz entprellt.
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const rev = project.rev;
    void project.doc;
    clearTimeout(timer);
    timer = setTimeout(() => load(rev), 250);
    return () => clearTimeout(timer);
  });

  let resizeTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void width;
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => draw(), 150);
  });

  $effect(() => () => task?.destroy());
</script>

<div class="pdf" bind:clientWidth={width} data-testid="pdf-view">
  <div class="bar">
    {#if status === "loading"}<span class="muted">{t("pdf.loading")}</span>
    {:else if status === "ok"}<span class="muted" data-testid="pdf-info">{pages === 1 ? t("pdf.page") : t("pdf.pages", { n: pages })}</span>
      <a href={`/pdx/pdf/${project.rev}`} target="_blank" rel="noopener">{t("pdf.open")}</a>
    {/if}
  </div>
  {#if status === "error"}
    <div class="error" role="alert">
      <strong>{t("pdf.error")}</strong>
      <pre>{error}</pre>
    </div>
  {/if}
  <div class="pages" bind:this={host}></div>
</div>

<style>
  .pdf {
    height: 100%;
    overflow: auto;
    background: var(--canvas);
  }
  .bar {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    gap: var(--space-3);
    align-items: center;
    padding: var(--space-1) var(--space-3);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    font-size: 12px;
  }
  .muted {
    color: var(--text-muted);
  }
  .pages {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-4);
  }
  .pages :global(canvas) {
    background: #fff;
    box-shadow: var(--shadow);
  }
  .error {
    margin: var(--space-4);
    color: var(--danger);
  }
  pre {
    white-space: pre-wrap;
    font-family: var(--font-mono);
    font-size: 12px;
  }
</style>
