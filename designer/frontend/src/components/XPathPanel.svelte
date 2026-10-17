<script lang="ts">
  import { untrack } from "svelte";
  import { TemplateAPI, type DataField, type Suggestion } from "../lib/api";
  import { label, propLabel, t } from "../lib/i18n";
  import { typeKey } from "../lib/data.svelte";
  import { node, typeOf, tpl } from "../lib/template.svelte";
  import { selection } from "../lib/selection.svelte";
  import { commit, edit, insert, setDraft, xp } from "../lib/xpath.svelte";
  import XPathInput from "./XPathInput.svelte";
  import XPathStatus from "./XPathStatus.svelte";

  // Folgt der Auswahl: gesetzte Ausdrücke zuerst, sonst der erste mögliche.
  $effect(() => {
    const ref = selection.ref;
    untrack(() => {
      const n = node(ref);
      if (!n || xp.ref === ref) return;
      const props = (typeOf(n)?.properties ?? []).filter((p) => p.kind === "xpath").map((p) => p.name);
      const prop = props.find((p) => n.attrs[p]) ?? props[0];
      if (prop) edit(ref, prop, n.attrs[prop] ?? "");
    });
  });

  const n = $derived(node(xp.ref));
  const props = $derived((typeOf(n)?.properties ?? []).filter((p) => p.kind === "xpath").map((p) => p.name));
  const current = $derived(n?.attrs[xp.prop] ?? "");
  const dirty = $derived(!!n && xp.draft.trim() !== current);

  let fields: DataField[] = $state.raw([]);
  let functions: Suggestion[] = $state.raw([]);
  $effect(() => {
    const ref = xp.ref;
    void tpl.snapshot?.rev;
    if (!ref) return;
    TemplateAPI.XPathFields(ref)
      .then((f) => (fields = f))
      .catch(() => (fields = []));
    TemplateAPI.CompleteXPath({ ref, expr: "", cursor: 0 })
      .then((c) => (functions = c.items.filter((i) => i.kind === "function")))
      .catch(() => (functions = []));
  });

  function switchProp(prop: string) {
    if (!n) return;
    xp.ref = "";
    edit(n.ref, prop, n.attrs[prop] ?? "");
  }
</script>

<div class="xpath-panel" data-testid="xpath-panel">
  {#if n && xp.prop}
    <div class="head">
      <strong>{label(n.type, n.name)}</strong>
      {#if !n.ref.startsWith("~")}<code>{n.ref}</code>{/if}
      <select aria-label={t("panel.xpath")} value={xp.prop} onchange={(e) => switchProp(e.currentTarget.value)}>
        {#each props as p (p)}<option value={p}>{propLabel(p)}</option>{/each}
      </select>
      <span class="ctx">{t("xpath.context")} <code data-testid="xpath-context">{xp.info?.context ?? ""}</code></span>
    </div>
    <div class="body">
      <div class="editor">
        <XPathInput ref={n.ref} prop={xp.prop} value={current} multiline status={false} label={propLabel(xp.prop)} placeholder={t("xpath.hint")} />
        <div class="actions">
          <div class="result">{#if xp.draft.trim()}<XPathStatus />{/if}</div>
          <!-- mousedown verhindert, dass der Editor den Fokus verliert und dabei schon übernimmt -->
          <button disabled={!dirty} onmousedown={(e) => e.preventDefault()} onclick={() => commit(current)}>{t("xpath.apply")}</button>
          <button disabled={!dirty} onmousedown={(e) => e.preventDefault()} onclick={() => setDraft(current)}>{t("xpath.revert")}</button>
        </div>
      </div>
      <div class="list" aria-label={t("xpath.fields")}>
        <h4>{t("xpath.fields")}</h4>
        <!-- mousedown, damit der Fokus im Editor bleibt -->
        {#each fields as f (f.path)}
          <button class="item" title={t(`data.type.${typeKey(f)}`)}
            onmousedown={(e) => {
              e.preventDefault();
              insert(f.path);
            }}>{f.path}{#if f.repeated}<em>{t("data.repeated")}</em>{:else if f.optional}<em>{t("data.optional")}</em>{/if}</button>
        {/each}
      </div>
      <div class="list" aria-label={t("xpath.functions")}>
        <h4>{t("xpath.functions")}</h4>
        {#each functions as f (f.label)}
          <button class="item fn" title={f.doc}
            onmousedown={(e) => {
              e.preventDefault();
              insert(f.insert);
            }}>{f.type}</button>
        {/each}
      </div>
    </div>
  {:else}
    <p class="muted">{t("xpath.none")}</p>
  {/if}
</div>

<style>
  .xpath-panel {
    padding: var(--space-2) var(--space-3);
    height: 100%;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .ctx {
    color: var(--text-muted);
    margin-left: auto;
  }
  code {
    font-size: 11px;
    color: var(--text-muted);
  }
  .body {
    display: grid;
    grid-template-columns: minmax(240px, 2fr) minmax(140px, 1fr) minmax(140px, 1fr);
    gap: var(--space-3);
    min-height: 0;
    flex: 1;
  }
  .actions {
    display: flex;
    gap: var(--space-1);
    align-items: flex-start;
    margin-top: 2px;
  }
  .result {
    flex: 1;
    min-width: 0;
  }
  .list {
    overflow: auto;
    min-height: 0;
    border-left: 1px solid var(--border);
    padding-left: var(--space-2);
  }
  h4 {
    margin: 0 0 4px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .item {
    display: block;
    width: 100%;
    text-align: left;
    border: 0;
    background: none;
    padding: 1px 4px;
    font-family: var(--font-mono);
    font-size: 12px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .item:hover {
    background: var(--accent-soft);
  }
  .item em {
    margin-left: 6px;
    font-family: var(--font-ui);
    color: var(--text-muted);
  }
  .muted {
    color: var(--text-muted);
  }
</style>
