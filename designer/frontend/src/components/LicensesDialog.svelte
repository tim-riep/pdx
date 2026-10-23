<script lang="ts">
  import Modal from "./Modal.svelte";
  import { AppAPI } from "../lib/api";
  import { t } from "../lib/i18n";

  let { onclose }: { onclose: () => void } = $props();

  type Entry = { name: string; license: string; text: string };
  let own: Entry | null = $state(null);
  let entries: Entry[] = $state([]);
  let full = $state("");
  let selected = $state(0);
  let filter = $state("");
  let copied = $state(false);

  AppAPI.Licenses().then((l) => {
    own = { name: l.name, license: "Apache-2.0", text: l.license + "\n\n" + l.notice };
    entries = (l.components ?? []).map((c) => ({ name: c.name, license: c.license, text: c.text }));
    full = l.full;
  });

  // Index 0 is the designer itself, then the third-party parts.
  const all = $derived(own ? [own, ...entries] : entries);
  const shown = $derived(
    all.map((e, i) => ({ e, i })).filter(({ e }) => {
      const q = filter.trim().toLowerCase();
      return !q || e.name.toLowerCase().includes(q) || e.license.toLowerCase().includes(q);
    }),
  );
  const current = $derived(all[selected]);

  async function copyAll() {
    try {
      await navigator.clipboard.writeText(full);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      copied = false;
    }
  }
</script>

<Modal title={t("licenses.title")} wide>
  <div class="licenses" data-testid="licenses">
    <p class="intro">{t("licenses.intro", { n: entries.length })}</p>
    <div class="split">
      <div class="side">
        <input type="search" placeholder={t("licenses.filter")} aria-label={t("licenses.filter")} bind:value={filter} data-testid="licenses-filter" />
        <ul role="listbox" aria-label={t("licenses.title")} data-testid="licenses-list">
          {#each shown as { e, i } (i)}
            <li>
              <button role="option" aria-selected={selected === i} class:active={selected === i} onclick={() => (selected = i)} data-component={e.name}>
                <span class="name">{i === 0 ? t("licenses.own") : e.name}</span>
                <span class="badge">{e.license || t("licenses.unknown")}</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
      <!-- Scrollable areas must be reachable by keyboard (WCAG 2.1.1). -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <pre class="text" data-testid="licenses-text" role="region" aria-label={current?.name ?? t("licenses.title")} tabindex="0">{current?.text ?? ""}</pre>
    </div>
  </div>
  {#snippet actions()}
    <button onclick={copyAll} disabled={!full} data-testid="licenses-copy">{copied ? t("licenses.copied") : t("licenses.copy")}</button>
    <button class="primary" onclick={onclose}>{t("licenses.close")}</button>
  {/snippet}
</Modal>

<style>
  .intro {
    margin: 0 0 var(--space-2);
    font-size: 12px;
    color: var(--text-muted);
  }
  .split {
    display: grid;
    grid-template-columns: 300px 1fr;
    gap: var(--space-3);
    height: 60vh;
  }
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 0;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: 4px;
    flex: 1;
  }
  li button {
    width: 100%;
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-2);
    text-align: left;
    border: none;
    border-radius: 0;
    background: none;
    padding: 4px var(--space-2);
    font-size: 12px;
  }
  li button.active {
    background: var(--accent-soft, var(--surface-2));
    font-weight: 600;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .badge {
    flex: none;
    font-size: 10px;
    color: var(--text-muted);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 0 4px;
  }
  .text {
    margin: 0;
    overflow: auto;
    font-size: 11px;
    line-height: 1.45;
    white-space: pre-wrap;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: var(--space-2);
  }
</style>
