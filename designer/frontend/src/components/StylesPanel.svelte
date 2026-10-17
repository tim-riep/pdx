<script lang="ts">
  import { TemplateAPI } from "../lib/api";
  import { propLabel, t } from "../lib/i18n";
  import { apply, tpl } from "../lib/template.svelte";
  import { select, selection } from "../lib/selection.svelte";
  import { addStyle, hex6, resolveColor, styles, tokenOp, tokens, type Token } from "../lib/tokens.svelte";

  let { onopen }: { onopen: () => void } = $props();

  const list = $derived(tokens());
  const groups = $derived([
    ["color", t("styles.colors"), list.filter((x) => x.kind === "color")],
    ["length", t("styles.lengths"), list.filter((x) => x.kind === "length")],
    ["font", t("styles.fonts"), list.filter((x) => x.kind === "font")],
  ] as const);
  const styleList = $derived(styles());

  let usage: Record<string, number | undefined> = $state({});
  $effect(() => {
    void tpl.snapshot?.rev;
    TemplateAPI.TokenUsage()
      .then((u) => (usage = u ?? {}))
      .catch(() => (usage = {}));
  });

  async function set(tok: Token, name: string, value: string, el?: HTMLInputElement | HTMLSelectElement) {
    if ((tok.attrs[name] ?? "") === value) return;
    const ok = await apply(t("cmd.setAttr", { prop: propLabel(name) }), { op: "setAttr", ref: tok.ref, name, value });
    if (!ok && el) el.value = tok.attrs[name] ?? "";
  }

  async function rename(tok: Token, el: HTMLInputElement) {
    const v = el.value.trim();
    if (v === tok.id) return;
    if (!(await tokenOp({ op: "rename", id: tok.id, newId: v }))) el.value = tok.id;
  }

  function key(e: KeyboardEvent) {
    const el = e.currentTarget as HTMLInputElement;
    if (e.key === "Enter") el.blur();
  }

  async function openStyle(ref: string) {
    select(ref, null, true);
    onopen();
  }

  const addLabel = { color: "styles.addColor", length: "styles.addLength", font: "styles.addFont" } as const;
</script>

<div class="styles" data-testid="styles">
  <h3>{t("styles.tokens")}</h3>
  {#if list.length === 0}<p class="muted">{t("styles.noTokens")}</p>{/if}
  {#each groups as [kind, title, items] (kind)}
    {#if items.length}
      <h4>{title}</h4>
      {#each items as tok (tok.ref)}
        <div class="token {kind}" data-token={tok.id}>
          {#if kind === "color"}
            <input type="color" class="swatch" title={t("styles.pick")} aria-label={t("styles.pick")}
              value={hex6(resolveColor(tok.attrs.value ?? ""))} disabled={(tok.attrs.value ?? "").startsWith("$")}
              onchange={(e) => set(tok, "value", e.currentTarget.value)} />
          {/if}
          <span class="dollar">$</span>
          <input class="name" value={tok.id} aria-label={t("styles.name")} spellcheck="false" onchange={(e) => rename(tok, e.currentTarget)} onkeydown={key} />
          {#if kind !== "font"}
            <input class="value" value={tok.attrs.value ?? ""} aria-label={t("styles.value")} spellcheck="false"
              onchange={(e) => set(tok, "value", e.currentTarget.value.trim(), e.currentTarget)} onkeydown={key} />
          {/if}
          <span class="used" title={t("styles.usedTitle", { n: usage[tok.id] ?? 0 })}>{t("styles.used", { n: usage[tok.id] ?? 0 })}</span>
          <button class="remove" title={t("styles.remove")} aria-label={t("styles.remove")} disabled={(usage[tok.id] ?? 0) > 0}
            onclick={() => tokenOp({ op: "remove", id: tok.id })}>×</button>
        </div>
        {#if kind === "font"}
          <div class="token font-details">
            <input class="family" value={tok.attrs.family ?? ""} aria-label={t("styles.family")} placeholder={t("styles.family")}
              onchange={(e) => set(tok, "family", e.currentTarget.value.trim(), e.currentTarget)} onkeydown={key} />
            <input class="size" value={tok.attrs.size ?? ""} aria-label={t("styles.size")} placeholder={t("styles.size")}
              onchange={(e) => set(tok, "size", e.currentTarget.value.trim(), e.currentTarget)} onkeydown={key} />
            <select aria-label={propLabel("fontWeight")} value={tok.attrs.weight ?? ""} onchange={(e) => set(tok, "weight", e.currentTarget.value, e.currentTarget)}>
              <option value=""></option><option value="normal">normal</option><option value="bold">bold</option>
            </select>
            <select aria-label={propLabel("fontStyle")} value={tok.attrs.style ?? ""} onchange={(e) => set(tok, "style", e.currentTarget.value, e.currentTarget)}>
              <option value=""></option><option value="normal">normal</option><option value="italic">italic</option>
            </select>
          </div>
        {/if}
      {/each}
    {/if}
  {/each}
  <div class="add">
    {#each ["color", "length", "font"] as const as kind (kind)}
      <button onclick={() => tokenOp({ op: "add", kind })}>+ {t(addLabel[kind])}</button>
    {/each}
  </div>

  <h3>{t("styles.styles")}</h3>
  {#if styleList.length === 0}<p class="muted">{t("styles.noStyles")}</p>{/if}
  <ul>
    {#each styleList as st (st.ref)}
      <li>
        <button class="style" class:selected={selection.ref === st.ref} onclick={() => openStyle(st.ref)}>
          <code>{st.attrs.id}</code>
          {#if st.attrs.parent}<span class="muted">{t("styles.parent", { parent: st.attrs.parent })}</span>{/if}
        </button>
      </li>
    {/each}
  </ul>
  <div class="add">
    <button onclick={async () => (await addStyle()) && onopen()}>+ {t("styles.addStyle")}</button>
  </div>
</div>

<style>
  .styles {
    padding: var(--space-2) var(--space-3);
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: var(--space-3) 0 var(--space-1);
  }
  h3:first-child {
    margin-top: 0;
  }
  h4 {
    margin: var(--space-2) 0 2px;
    font-size: 12px;
    font-weight: 600;
  }
  .muted {
    color: var(--text-muted);
  }
  .token {
    display: flex;
    align-items: center;
    gap: 4px;
    margin: 2px 0;
  }
  .token input,
  .token select {
    min-width: 0;
    font-size: 12px;
  }
  .swatch {
    width: 24px;
    height: 22px;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: none;
    flex: none;
  }
  .dollar {
    color: var(--text-muted);
    font-family: var(--font-mono);
  }
  .name {
    font-family: var(--font-mono);
    flex: 1 1 70px;
  }
  .value {
    font-family: var(--font-mono);
    flex: 1 1 70px;
  }
  .font-details {
    padding-left: 12px;
    margin-bottom: 4px;
  }
  .family {
    flex: 3 1 80px;
  }
  .size {
    flex: 1 1 40px;
  }
  .font-details select {
    flex: 1 1 52px;
  }
  .used {
    color: var(--text-muted);
    font-size: 11px;
    width: 22px;
    text-align: right;
    flex: none;
  }
  .remove {
    border: 0;
    background: none;
    padding: 0 4px;
    flex: none;
  }
  .add {
    display: flex;
    gap: var(--space-1);
    margin-top: var(--space-2);
    flex-wrap: wrap;
  }
  .add button {
    font-size: 12px;
    padding: 2px 8px;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .style {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    width: 100%;
    text-align: left;
    border: 0;
    background: none;
    padding: 2px 4px;
    border-radius: 3px;
  }
  .style:hover,
  .style.selected {
    background: var(--accent-soft);
  }
</style>
