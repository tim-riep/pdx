<script lang="ts">
  import type { Property } from "../lib/api";
  import { t, label, propLabel } from "../lib/i18n";
  import { node, typeOf } from "../lib/template.svelte";
  import { selection } from "../lib/selection.svelte";

  let query = $state("");
  const n = $derived(node(selection.ref));
  const type = $derived(typeOf(n));

  const groups = $derived.by(() => {
    const q = query.trim().toLowerCase();
    const out = new Map<string, Property[]>();
    for (const p of type?.properties ?? []) {
      if (q && !p.name.toLowerCase().includes(q) && !propLabel(p.name).toLowerCase().includes(q)) continue;
      const list = out.get(p.group) ?? [];
      list.push(p);
      out.set(p.group, list);
    }
    return [...out];
  });
</script>

<div class="inspector" data-testid="inspector">
  {#if n && type}
    <header>
      <strong>{label(n.type, n.name)}</strong>
      {#if !n.ref.startsWith("~")}<code>{n.ref}</code>{/if}
    </header>
    <input class="search" type="search" placeholder={t("inspector.search")} aria-label={t("inspector.search")} bind:value={query} />
    {#each groups as [g, props] (g)}
      <section>
        <h3>{t(`group.${g}` as "group.general")}</h3>
        {#each props as p (p.name)}
          <label class="prop" data-prop={p.name}>
            <span class="label" title={p.name}>{propLabel(p.name)}{#if p.required}<sup>*</sup>{/if}</span>
            {#if p.kind === "enum"}
              <select disabled value={n.attrs[p.name] ?? ""}>
                <option value="">{p.default ? t("inspector.default", { value: p.default }) : ""}</option>
                {#each p.enum ?? [] as v}<option value={v}>{v}</option>{/each}
              </select>
            {:else if p.kind === "bool"}
              <input type="checkbox" disabled checked={(n.attrs[p.name] ?? p.default) === "true"} />
            {:else}
              <input readonly value={n.attrs[p.name] ?? ""} placeholder={p.default ?? ""} class:mono={p.kind === "xpath" || p.kind === "id" || p.kind === "ref"} />
            {/if}
          </label>
        {/each}
      </section>
    {/each}
  {:else}
    <p class="muted">{t("inspector.none")}</p>
  {/if}
</div>

<style>
  .inspector {
    padding: var(--space-2) var(--space-3);
  }
  header {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    margin-bottom: var(--space-2);
  }
  code {
    color: var(--text-muted);
    font-size: 11px;
  }
  .search {
    width: 100%;
    box-sizing: border-box;
    margin-bottom: var(--space-2);
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  .prop {
    display: grid;
    grid-template-columns: 110px 1fr;
    align-items: center;
    gap: var(--space-2);
    margin: 2px 0;
  }
  .prop .label {
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .prop input:not([type="checkbox"]),
  .prop select {
    width: 100%;
    box-sizing: border-box;
    min-width: 0;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  sup {
    color: var(--danger);
  }
  .muted {
    color: var(--text-muted);
  }
</style>
