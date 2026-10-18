<script lang="ts">
  import Modal from "./Modal.svelte";
  import { AppAPI } from "../lib/api";
  import { t } from "../lib/i18n";

  let { version, onclose }: { version: string; onclose: () => void } = $props();
  let notices = $state("");
  AppAPI.Notices().then((n) => (notices = n));
</script>

<Modal title={t("about.title")}>
  <div class="about" data-testid="about">
    <p>{t("about.text")}</p>
    <p class="muted">{version}</p>
    <h3>{t("about.notices")}</h3>
    <pre data-testid="notices">{notices}</pre>
  </div>
  {#snippet actions()}
    <button class="primary" onclick={onclose}>{t("about.close")}</button>
  {/snippet}
</Modal>

<style>
  .about {
    width: min(520px, 80vw);
  }
  .muted {
    color: var(--text-muted);
    font-size: 12px;
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  pre {
    max-height: 40vh;
    overflow: auto;
    font-size: 11px;
    line-height: 1.4;
    white-space: pre-wrap;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: var(--space-2);
    margin: 0;
  }
</style>
