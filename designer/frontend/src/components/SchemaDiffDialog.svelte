<script lang="ts">
  import Modal from "./Modal.svelte";
  import { t } from "../lib/i18n";
  import { applyChange, cancelChange, sd } from "../lib/schemadiff.svelte";

  const groups = $derived(
    (["renamed", "removed", "changed", "added"] as const)
      .map((k) => [k, (sd.diff?.changes ?? []).filter((c) => c.kind === k)] as const)
      .filter(([, l]) => l.length > 0),
  );
  const refs = (n: number) => (n === 1 ? t("schemadiff.ref") : t("schemadiff.refs", { n }));
</script>

<Modal title={t("schemadiff.title")}>
  <div class="diff" data-testid="schema-diff">
    {#if groups.length === 0}
      <p>{t("schemadiff.none")}</p>
    {:else}
      <p>{t("schemadiff.intro")}</p>
      {#each groups as [kind, list] (kind)}
        <h3>{t(`schemadiff.${kind}`)}</h3>
        <ul>
          {#each list as c (c.path)}
            <li data-change={c.path}>
              <code>{c.path}</code>
              {#if kind === "renamed"}
                → <code>{c.to}</code>
                <label class="switch"><input type="checkbox" bind:checked={sd.rename[c.path]} /> {t("schemadiff.switch")}</label>
              {/if}
              {#if kind === "changed"}<span class="detail">{c.detail}</span>{/if}
              {#if kind === "added" && c.required}<span class="req">{t("schemadiff.required")}</span>{/if}
              {#if c.refs.length}<span class="refs" title={c.refs.join(", ")}>{refs(c.refs.length)}</span>{/if}
            </li>
          {/each}
        </ul>
      {/each}
    {/if}
  </div>
  {#snippet actions()}
    <button onclick={cancelChange}>{t("schemadiff.cancel")}</button>
    <button class="primary" onclick={applyChange}>{t("schemadiff.apply")}</button>
  {/snippet}
</Modal>

<style>
  .diff {
    max-height: 50vh;
    overflow: auto;
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    padding: 2px 0;
    display: flex;
    flex-wrap: wrap;
    gap: 4px var(--space-2);
    align-items: baseline;
  }
  code {
    font-size: 12px;
  }
  .switch {
    display: inline-flex;
    gap: 4px;
    align-items: center;
    font-size: 12px;
  }
  .detail,
  .refs {
    color: var(--text-muted);
    font-size: 12px;
  }
  .req {
    color: var(--warning);
    font-size: 12px;
  }
</style>
