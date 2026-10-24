<script lang="ts">
  import Modal from "./Modal.svelte";
  import { t } from "../lib/i18n";
  import { openRelease, sanitizeNotes, skipRelease, startUpdateCheck, update } from "../lib/update.svelte";

  // Toolbar: a button for a newer release and the dialog with its notes,
  // which opens by itself once per start. The check itself is switched in
  // the About dialog.
  startUpdateCheck();

  const info = $derived(update.info);
  const notes = $derived(sanitizeNotes(info?.notesHtml));
  const published = $derived(info?.publishedAt ? new Date(info.publishedAt).toLocaleDateString("de-DE") : "");

  // A link in the notes never opens inside the app window. Release pages of
  // pdx open in the browser, the service refuses every other address.
  function onNotesClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest("a");
    if (!a) return;
    e.preventDefault();
    void openRelease(a.getAttribute("href") ?? "");
  }
</script>

{#if info?.newer}
  <button
    class="primary"
    data-testid="update-open"
    title={t("update.buttonTitle", { current: info.current, latest: info.latest })}
    onclick={() => (update.open = true)}>{t("update.button", { latest: info.latest })}</button
  >
{/if}

{#if update.open && info?.newer}
  <Modal title={t("update.title", { version: info.latest })}>
    <div class="update-dialog" data-testid="update-dialog">
      <p class="meta">
        {published ? t("update.haveReleased", { current: info.current, date: published }) : t("update.have", { current: info.current })}
      </p>
      {#if notes}
        <!-- eslint-disable-next-line svelte/no-at-html-tags -->
        <div class="notes" role="presentation" onclick={onNotesClick}>{@html notes}</div>
      {:else}
        <p class="notes empty">{t("update.noNotes")}</p>
      {/if}
      {#if update.error}<p class="error" role="alert">{update.error}</p>{/if}
    </div>
    {#snippet actions()}
      <button data-testid="update-skip" title={t("update.skipTitle")} onclick={skipRelease}>{t("update.skip")}</button>
      <button data-testid="update-later" onclick={() => (update.open = false)}>{t("update.later")}</button>
      <button class="primary" data-testid="update-download" onclick={() => openRelease()}>{t("update.download")}</button>
    {/snippet}
  </Modal>
{/if}

<style>
  .update-dialog {
    width: min(520px, 80vw);
  }
  .meta {
    color: var(--text-muted);
    font-size: 12px;
    margin-top: 0;
  }
  .notes {
    max-height: 50vh;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0 var(--space-3);
    font-size: 13px;
  }
  .notes.empty {
    padding: var(--space-3);
    color: var(--text-muted);
  }
  .notes :global(table) {
    border-collapse: collapse;
    font-size: 12px;
  }
  .notes :global(th),
  .notes :global(td) {
    border: 1px solid var(--border);
    padding: 2px 6px;
    text-align: left;
  }
  .notes :global(pre) {
    overflow: auto;
  }
  .error {
    color: var(--danger);
  }
</style>
