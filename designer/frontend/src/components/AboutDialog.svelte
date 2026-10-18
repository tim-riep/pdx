<script lang="ts">
  import Modal from "./Modal.svelte";
  import { t } from "../lib/i18n";

  let { version, onclose, onlicenses }: { version: string; onclose: () => void; onlicenses: () => void } = $props();
</script>

<Modal title={t("about.title")}>
  <div class="about" data-testid="about">
    <p>{t("about.text")}</p>
    <p class="muted">{version}</p>
  </div>
  {#snippet actions()}
    <button onclick={onlicenses} data-testid="about-licenses">{t("about.licenses")}</button>
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
</style>
