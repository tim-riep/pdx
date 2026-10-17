<script lang="ts">
  import { untrack } from "svelte";
  import type { Suggestion } from "../lib/api";
  import { t } from "../lib/i18n";
  import { typeKey } from "../lib/data.svelte";
  import { commit, complete, edit, isActive, setDraft, xp } from "../lib/xpath.svelte";
  import XPathStatus from "./XPathStatus.svelte";

  let {
    ref,
    prop,
    value,
    multiline = false,
    label = "",
    placeholder = "",
    status = true,
  }: { ref: string; prop: string; value: string; multiline?: boolean; label?: string; placeholder?: string; status?: boolean } = $props();

  const uid = $props.id();
  let el: HTMLInputElement | HTMLTextAreaElement | undefined = $state();
  let items: Suggestion[] = $state.raw([]);
  let range = { from: 0, to: 0 };
  let index = $state(0);
  let focused = $state(false);

  const active = $derived(isActive(ref, prop));
  const shown = $derived(active ? xp.draft : value);

  // Ändert sich der Wert im Modell (Undo, anderer Editor), folgt der Entwurf,
  // solange hier niemand tippt.
  $effect(() => {
    const v = value;
    untrack(() => {
      if (active && !focused && xp.draft !== v) setDraft(v);
    });
  });

  async function suggest() {
    if (!el) return;
    const cursor = el.selectionStart ?? xp.draft.length;
    const c = await complete(ref, xp.draft, cursor);
    if (!c || !focused) return;
    items = c.items;
    range = { from: c.from, to: c.to };
    index = 0;
  }

  function accept(it: Suggestion) {
    if (!el) return;
    const v = xp.draft.slice(0, range.from) + it.insert + xp.draft.slice(range.to);
    setDraft(v);
    el.value = v;
    const pos = range.from + it.insert.length;
    el.setSelectionRange(pos, pos);
    xp.caret = pos;
    items = [];
  }

  // Wörter, Schrägstriche und @ öffnen die Vorschläge.
  function caret() {
    if (el && active) xp.caret = el.selectionStart ?? xp.draft.length;
  }

  function oninput(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    setDraft(target.value);
    caret();
    const ch = target.value[(target.selectionStart ?? 1) - 1] ?? "";
    if (/[\w:@/.-]/.test(ch)) suggest();
    else items = [];
  }

  async function done() {
    items = [];
    await commit(value);
  }

  function onkeydown(e: KeyboardEvent) {
    if (items.length) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        index = (index + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length;
        e.preventDefault();
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        accept(items[index]);
        e.preventDefault();
        return;
      }
      if (e.key === "Escape") {
        items = [];
        e.preventDefault();
        e.stopPropagation();
        return;
      }
    }
    if (e.key === " " && e.ctrlKey) {
      suggest();
      e.preventDefault();
      return;
    }
    if (e.key === "Enter" && (!multiline || e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      if (multiline) done();
      else el?.blur();
      return;
    }
    if (e.key === "Escape") {
      setDraft(value);
      if (el) el.value = value;
      e.stopPropagation();
      el?.blur();
    }
  }

  function onfocus() {
    focused = true;
    edit(ref, prop, value);
  }

  function onblur() {
    focused = false;
    done();
  }

  const typeLabel = (it: Suggestion) => {
    if (it.kind === "function") return it.type;
    if (it.kind === "step") return t("xpath.parent");
    const parts = [t(`data.type.${typeKey(it)}`)];
    if (it.repeated) parts.push(t("data.repeated"));
    else if (it.optional) parts.push(t("data.optional"));
    return parts.join(", ");
  };
</script>

<div class="xpath" class:multiline>
  {#if multiline}
    <textarea bind:this={el} value={shown} {placeholder} spellcheck="false" aria-label={label} rows="3" aria-autocomplete="list"
      aria-controls={uid} {oninput} {onkeydown} {onfocus} {onblur} onkeyup={caret} onclick={caret}></textarea>
  {:else}
    <input bind:this={el} value={shown} {placeholder} spellcheck="false" aria-label={label} role="combobox" aria-autocomplete="list"
      aria-expanded={items.length > 0} aria-controls={uid} {oninput} {onkeydown} {onfocus} {onblur} onkeyup={caret} onclick={caret} />
  {/if}
  {#if items.length}
    <ul id={uid} class="menu" role="listbox" aria-label={t("xpath.suggestions")} data-testid="xpath-menu">
      {#each items as it, i (it.kind + it.label)}
        <!-- mousedown statt click, damit das Feld den Fokus behält -->
        <li role="option" aria-selected={i === index} class:sel={i === index} class={it.kind}
          onmousedown={(e) => {
            e.preventDefault();
            accept(it);
          }}>
          <span class="name">{it.label}</span>
          <span class="type">{typeLabel(it)}</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#if status && active && xp.draft.trim()}
    <XPathStatus compact={!multiline} />
  {/if}
</div>

<style>
  .xpath {
    position: relative;
    min-width: 0;
  }
  input,
  textarea {
    width: 100%;
    box-sizing: border-box;
    font-family: var(--font-mono);
    font-size: 12px;
  }
  textarea {
    resize: vertical;
  }
  .menu {
    position: absolute;
    z-index: 20;
    left: 0;
    right: 0;
    top: 100%;
    margin: 2px 0 0;
    padding: 2px;
    list-style: none;
    max-height: 220px;
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
  }
  .multiline .menu {
    top: auto;
    bottom: 100%;
  }
  li {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    padding: 2px 6px;
    border-radius: 3px;
    cursor: pointer;
    font-size: 12px;
  }
  li.sel {
    background: var(--accent-soft);
  }
  .name {
    font-family: var(--font-mono);
    white-space: nowrap;
  }
  .type {
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  li.function .name {
    font-style: italic;
  }
</style>
