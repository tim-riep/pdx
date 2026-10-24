<script lang="ts">
  import Modal from "./Modal.svelte";
  import { t } from "../lib/i18n";
  import { setUpdateCheck, update } from "../lib/update.svelte";

  let { version, onclose, onlicenses }: { version: string; onclose: () => void; onlicenses: () => void } = $props();
</script>

<Modal title={t("about.title")}>
  <div class="about" data-testid="about">
    <p>{t("about.text")}</p>
    <p class="muted">{version}</p>
    <label class="check">
      <input
        type="checkbox"
        data-testid="update-check"
        checked={update.settings.check}
        onchange={(e) => setUpdateCheck((e.currentTarget as HTMLInputElement).checked)}
      />
      {t("update.setting")}
    </label>
    <p class="muted">{t("update.settingHint")}</p>
    {#if update.error}<p class="error" role="alert">{update.error}</p>{/if}
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
  .check {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .error {
    color: var(--danger);
  }
</style>
