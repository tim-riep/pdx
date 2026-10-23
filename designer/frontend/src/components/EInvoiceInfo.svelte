<script lang="ts">
  // State of the e-invoice for the einvoice element: level detected from the data
  // of the preview, embedded file, relationship and whether the profile fits.
  import { RenderAPI, message } from "../lib/api";
  import { t } from "../lib/i18n";
  import { project } from "../lib/project.svelte";
  import { render } from "../lib/render.svelte";

  type Info = Awaited<ReturnType<typeof RenderAPI.EInvoice>>;
  let info = $state<Info | null>(null);
  let error = $state("");

  $effect(() => {
    // Again after every change and for another sample.
    void project.rev;
    void render.manifest?.sample;
    RenderAPI.EInvoice().then(
      (i) => ((info = i), (error = "")),
      (e) => (error = message(e)),
    );
  });
</script>

<section class="einvoice" data-testid="einvoice-info">
  <h3>{t("einvoice.title")}</h3>
  {#if error}
    <p class="bad">{error}</p>
  {:else if info}
    {#if info.profileError}<p class="bad" data-testid="einvoice-profile">{info.profileError}</p>{/if}
    {#if info.error}
      <p class="bad">{info.error}</p>
    {:else}
      <dl>
        <dt>{t("einvoice.level")}</dt><dd data-testid="einvoice-level">{info.level}</dd>
        <dt>{t("einvoice.file")}</dt><dd><code>{info.fileName}</code></dd>
        <dt>{t("einvoice.relationship")}</dt>
        <dd>{info.relationship}{#if !info.overridden}{" "}<span class="muted">({t("einvoice.byLevel")})</span>{/if}</dd>
      </dl>
    {/if}
    <p class="muted">{info.sample ? t("einvoice.from", { file: info.sample.replace(/^samples\//, "") }) : t("einvoice.noSample")}</p>
  {/if}
</section>

<style>
  .einvoice {
    margin: var(--space-2) 0;
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  dl {
    display: grid;
    grid-template-columns: 110px 1fr;
    gap: 2px var(--space-2);
    margin: 0;
  }
  dt {
    color: var(--text-muted);
  }
  dd {
    margin: 0;
  }
  .bad {
    color: var(--danger);
    margin: var(--space-1) 0;
  }
  .muted {
    color: var(--text-muted);
    font-size: 11px;
  }
  code {
    font-size: 12px;
  }
</style>
