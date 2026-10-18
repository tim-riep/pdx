<script lang="ts">
  import type { Property } from "../lib/api";
  import { t, label, propLabel, enumLabel, help, profiles, profileLabel, profileDesc } from "../lib/i18n";
  import { apply, node, typeOf } from "../lib/template.svelte";
  import { selection } from "../lib/selection.svelte";
  import TableTools from "./TableTools.svelte";
  import XPathInput from "./XPathInput.svelte";
  import PresenceEditor from "./PresenceEditor.svelte";
  import ContentEditor from "./ContentEditor.svelte";
  import AlignTools from "./AlignTools.svelte";
  import EInvoiceInfo from "./EInvoiceInfo.svelte";
  import { resolveColor, tokens } from "../lib/tokens.svelte";

  let query = $state("");
  const tokenList = $derived(tokens());
  const listOf = (kind: string) => (kind === "color" ? "tok-color" : kind === "length" || kind === "lengths" ? "tok-length" : undefined);
  const n = $derived(node(selection.ref));
  const type = $derived(typeOf(n));

  // commit schickt die Änderung als Befehl. Scheitert er, zeigt das Feld
  // wieder den Wert aus dem Modell.
  async function commit(p: Property, el: HTMLInputElement | HTMLSelectElement, value: string) {
    if (!n || (n.attrs[p.name] ?? "") === value) return;
    const ok = await apply(t("cmd.setAttr", { prop: propLabel(p.name) }), { op: "setAttr", ref: n.ref, name: p.name, value });
    if (!ok) {
      if (el instanceof HTMLInputElement && el.type === "checkbox") el.checked = (n.attrs[p.name] ?? p.default) === "true";
      else el.value = n.attrs[p.name] ?? "";
    }
  }

  function key(p: Property, e: KeyboardEvent) {
    const el = e.currentTarget as HTMLInputElement;
    if (e.key === "Enter") el.blur();
    if (e.key === "Escape") {
      el.value = n?.attrs[p.name] ?? "";
      el.blur();
      e.stopPropagation();
    }
  }

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
    <TableTools {n} />
    {#if n.name === "einvoice"}<EInvoiceInfo />{/if}
    <AlignTools {n} />
    {#if type.text}
      <section class="content" data-testid="inspector-content">
        <h3>{t("content.title")}</h3>
        {#key n.ref}<ContentEditor ref={n.ref} />{/key}
        <p class="hint">{type.mixed ? t("content.hint") : t("content.hintPlain")}</p>
      </section>
    {/if}
    <input class="search" type="search" placeholder={t("inspector.search")} aria-label={t("inspector.search")} bind:value={query} />
    {#each groups as [g, props] (g)}
      <section>
        <h3>{t(`group.${g}` as "group.general")}</h3>
        {#each props as p (p.name)}
          {#if p.kind === "xpath"}
            <div class="prop" data-prop={p.name}>
              <span class="label" title={p.name}>{propLabel(p.name)}{#if p.required}<sup>*</sup>{/if}</span>
              {#if p.name === "presence"}
                <PresenceEditor {n} />
              {:else}
                <XPathInput ref={n.ref} prop={p.name} value={n.attrs[p.name] ?? ""} label={propLabel(p.name)} placeholder={p.default ?? ""} />
              {/if}
              {#if help(p.name)}<p class="help">{help(p.name)}</p>{/if}
            </div>
          {:else}
          <label class="prop" data-prop={p.name}>
            <span class="label" title={p.name}>{propLabel(p.name)}{#if p.required}<sup>*</sup>{/if}</span>
            {#if p.kind === "enum"}
              <select value={n.attrs[p.name] ?? ""} onchange={(e) => commit(p, e.currentTarget, e.currentTarget.value)}>
                <option value="">{p.default ? t("inspector.default", { value: enumLabel(p.name, p.default) }) : ""}</option>
                {#each p.enum ?? [] as v}<option value={v}>{enumLabel(p.name, v)}</option>{/each}
              </select>
            {:else if p.kind === "profile"}
              {@const cur = n.attrs[p.name] ?? ""}
              <select value={cur} data-testid="profile-select" onchange={(e) => commit(p, e.currentTarget, e.currentTarget.value)}>
                <option value="">{t("inspector.default", { value: profileLabel("pdfa-3b") })}</option>
                {#each profiles as v}<option value={v}>{profileLabel(v)}</option>{/each}
                {#if cur && !profiles.includes(cur)}<option value={cur}>{profileLabel(cur)}</option>{/if}
              </select>
            {:else if p.kind === "font"}
              <select value={n.attrs[p.name] ?? ""} onchange={(e) => commit(p, e.currentTarget, e.currentTarget.value)}>
                <option value="">{t("tokens.none")}</option>
                {#each tokenList.filter((x) => x.kind === "font") as tok (tok.ref)}<option value={"$" + tok.id}>${tok.id}</option>{/each}
              </select>
            {:else if p.kind === "bool"}
              <input type="checkbox" checked={(n.attrs[p.name] ?? p.default) === "true"}
                onchange={(e) => commit(p, e.currentTarget, e.currentTarget.checked ? "true" : "false")} />
            {:else}
              <span class="with-swatch">
                {#if p.kind === "color" && n.attrs[p.name]}
                  <span class="swatch" style:background={resolveColor(n.attrs[p.name] ?? "", tokenList)} aria-hidden="true"></span>
                {/if}
                <input value={n.attrs[p.name] ?? ""} placeholder={p.default ?? ""} spellcheck="false" list={listOf(p.kind)}
                  class:mono={p.kind === "id" || p.kind === "ref"}
                  onchange={(e) => commit(p, e.currentTarget, e.currentTarget.value.trim())} onkeydown={(e) => key(p, e)} />
              </span>
            {/if}
          </label>
          {#if p.kind === "profile"}<p class="help" data-testid="profile-desc">{profileDesc(n.attrs[p.name] || "pdfa-3b")}</p>{/if}
          {#if p.kind !== "profile" && help(p.name)}<p class="help">{help(p.name)}</p>{/if}
          {/if}
        {/each}
      </section>
    {/each}
  {:else}
    <p class="muted">{t("inspector.none")}</p>
  {/if}
  <!-- Vorschläge für Verweise auf Tokens -->
  <datalist id="tok-color">{#each tokenList.filter((x) => x.kind === "color") as tok (tok.ref)}<option value={"$" + tok.id}>{tok.attrs.value}</option>{/each}</datalist>
  <datalist id="tok-length">{#each tokenList.filter((x) => x.kind === "length") as tok (tok.ref)}<option value={"$" + tok.id}>{tok.attrs.value}</option>{/each}</datalist>
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
  div.prop {
    align-items: start;
  }
  div.prop .label {
    padding-top: 3px;
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
  .with-swatch {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }
  .with-swatch input {
    flex: 1;
  }
  .swatch {
    width: 14px;
    height: 14px;
    border-radius: 3px;
    border: 1px solid var(--border);
    flex: none;
  }
  .content {
    margin-bottom: var(--space-2);
  }
  .content h3 {
    margin-top: 0;
  }
  .help {
    margin: 0 0 var(--space-2) calc(110px + var(--space-2));
    font-size: 11px;
    color: var(--text-muted);
  }
  div.prop .help {
    grid-column: 2;
    margin: 0;
  }
  .hint {
    margin: 2px 0 0;
    font-size: 11px;
    color: var(--text-muted);
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
