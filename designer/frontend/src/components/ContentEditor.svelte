<script lang="ts">
  // Text with embedded fields as chips. The text is directly
  // editable, fields and other embedded elements sit in it as non
  // editable chips. Applied as one command on leaving
  // or with Enter.
  import { onMount, untrack } from "svelte";
  import { TemplateAPI, message, type DataField, type Segment } from "../lib/api";
  import { label, t } from "../lib/i18n";
  import { node, tpl, typeOf } from "../lib/template.svelte";
  import { ui } from "../lib/project.svelte";
  import { DATA_MIME } from "../lib/data.svelte";

  let {
    ref,
    autofocus = false,
    ondone,
  }: { ref: string; autofocus?: boolean; ondone?: () => void } = $props();

  let el: HTMLDivElement | undefined = $state();
  const n = $derived(node(ref));
  const type = $derived(typeOf(n));
  const plain = $derived(!type?.mixed);
  const allowsBr = $derived(!!type?.children.some((c) => c.name === "br"));

  // Suggestions after @: fields relative to the context of the text.
  let fields: DataField[] = $state.raw([]);
  let query = $state<string | null>(null);
  let index = $state(0);
  // Exact matches first, then matches at the start, then the rest.
  const matches = $derived.by(() => {
    if (query === null) return [];
    const q = query.toLowerCase();
    const rank = (p: string) => (p === q ? 0 : p.startsWith(q) ? 1 : 2);
    return fields
      .filter((f) => f.path.toLowerCase().includes(q))
      .sort((a, b) => rank(a.path.toLowerCase()) - rank(b.path.toLowerCase()))
      .slice(0, 12);
  });

  let focused = false;
  let dirty = false;

  function chipText(r: string): string {
    const c = node(r);
    if (!c) return r;
    if (c.name === "field") return c.attrs.bind ?? "";
    if (c.name === "pageNumber") return c.attrs.pattern ?? label(c.type, c.name);
    return label(c.type, c.name);
  }

  function chip(attrs: { ref?: string; bind?: string }): HTMLSpanElement {
    const s = document.createElement("span");
    s.className = "chip";
    s.contentEditable = "false";
    if (attrs.ref) {
      s.dataset.ref = attrs.ref;
      s.textContent = chipText(attrs.ref);
    } else if (attrs.bind) {
      s.dataset.bind = attrs.bind;
      s.textContent = attrs.bind;
    }
    s.title = t("content.chip", { el: s.textContent ?? "" });
    return s;
  }

  // render builds the content from the read cache.
  function render() {
    if (!el || !n) return;
    el.replaceChildren();
    for (const it of n.children) {
      if (it.ref) {
        const c = node(it.ref);
        if (c?.name === "br") el.append(document.createElement("br"));
        else el.append(chip({ ref: it.ref }));
      } else if (it.text) el.append(document.createTextNode(it.text));
    }
    dirty = false;
  }

  // Rebuild when the content changes in the model and nobody is typing here.
  $effect(() => {
    void n?.children;
    untrack(() => {
      if (!focused) render();
    });
  });

  // Chromium on Linux and Windows puts the cursor with End or arrows
  // into a field too (contenteditable=false), where every input is
  // lost. A cursor in the field is therefore moved behind it.
  function caretOutOfChip() {
    const sel = getSelection();
    const a = sel?.anchorNode;
    if (!focused || !sel || !sel.isCollapsed || !a || !el?.contains(a)) return;
    const chipEl = (a.nodeType === Node.ELEMENT_NODE ? (a as Element) : a.parentElement)?.closest(".chip");
    if (!chipEl || !el.contains(chipEl)) return;
    const r = document.createRange();
    r.setStartAfter(chipEl);
    r.collapse(true);
    sel.removeAllRanges();
    sel.addRange(r);
  }

  onMount(() => {
    document.addEventListener("selectionchange", caretOutOfChip);
    return () => document.removeEventListener("selectionchange", caretOutOfChip);
  });

  onMount(() => {
    if (autofocus && el) {
      el.focus();
      const r = document.createRange();
      r.selectNodeContents(el);
      r.collapse(false);
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(r);
    }
  });

  // segments reads the content back: text, chips and line breaks.
  function segments(): Segment[] {
    const out: Segment[] = [];
    const text = (s: string) => {
      const last = out[out.length - 1];
      if (last && !last.ref && !last.bind) last.text += s;
      else out.push({ text: s, ref: "", bind: "" });
    };
    const walk = (parent: Node) => {
      parent.childNodes.forEach((c, i) => {
        if (c.nodeType === Node.TEXT_NODE) text((c.textContent ?? "").replace(/ /g, " "));
        else if (c instanceof HTMLBRElement) {
          // The browser adds a trailing br itself.
          if (!(parent === el && i === parent.childNodes.length - 1)) text("\n");
        } else if (c instanceof HTMLElement && c.classList.contains("chip")) {
          out.push({ text: "", ref: c.dataset.ref ?? "", bind: c.dataset.bind ?? "" });
        } else if (c instanceof HTMLElement) {
          // div or p from the browser on a line break
          if (out.length) text("\n");
          walk(c);
        }
      });
    };
    if (el) walk(el);
    return out.filter((s) => s.ref || s.bind || s.text);
  }

  async function commit() {
    if (!dirty || !n) return;
    dirty = false;
    try {
      const res = await TemplateAPI.SetContent({ ref: n.ref, segments: segments() });
      tpl.history = res.history;
      ui.error = "";
    } catch (e) {
      ui.error = message(e);
      render();
    }
  }

  function insertNode(x: Node) {
    const sel = getSelection();
    if (!el || !sel || !sel.rangeCount || !el.contains(sel.anchorNode)) {
      el?.append(x);
    } else {
      const r = sel.getRangeAt(0);
      r.deleteContents();
      r.insertNode(x);
      r.setStartAfter(x);
      r.collapse(true);
      sel.removeAllRanges();
      sel.addRange(r);
    }
    dirty = true;
  }

  // atQuery returns the text after the last @ before the cursor.
  function atQuery(): string | null {
    const sel = getSelection();
    const a = sel?.anchorNode;
    if (!a || a.nodeType !== Node.TEXT_NODE || !el?.contains(a)) return null;
    const before = (a.textContent ?? "").slice(0, sel!.anchorOffset);
    const m = /@([\w:./@-]*)$/.exec(before);
    return m ? m[1] : null;
  }

  // loading prevents parallel queries of the fields while typing
  // continues after the @.
  let loading: Promise<void> | null = null;

  async function oninput() {
    dirty = true;
    if (plain) return;
    if (atQuery() !== null && query === null) {
      loading ??= TemplateAPI.XPathFields(ref)
        .then((f) => void (fields = f ?? []))
        .catch(() => void (fields = []))
        .finally(() => (loading = null));
      await loading;
    }
    // After waiting, the current input applies, not the one from the start of the
    // query, otherwise the menu would stay on an older state.
    query = atQuery();
    index = 0;
  }

  function pick(f: DataField) {
    const sel = getSelection();
    const a = sel?.anchorNode;
    if (a && a.nodeType === Node.TEXT_NODE && query !== null) {
      // remove @ and search text
      const off = sel!.anchorOffset;
      const start = off - query.length - 1;
      const r = document.createRange();
      r.setStart(a, Math.max(0, start));
      r.setEnd(a, off);
      sel!.removeAllRanges();
      sel!.addRange(r);
    }
    insertNode(chip({ bind: f.path }));
    query = null;
  }

  function onkeydown(e: KeyboardEvent) {
    // selectionchange arrives asynchronously, check again here before
    // a character is inserted.
    caretOutOfChip();
    if (query !== null && matches.length) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        index = (index + (e.key === "ArrowDown" ? 1 : matches.length - 1)) % matches.length;
        e.preventDefault();
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        pick(matches[index]);
        return;
      }
    }
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      if (query !== null) {
        query = null;
        return;
      }
      dirty = false;
      render();
      el?.blur();
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      if (e.shiftKey && allowsBr) {
        insertNode(document.createElement("br"));
        // A br at the end needs a second one, so the cursor becomes visible.
        if (el?.lastChild instanceof HTMLBRElement) el.append(document.createElement("br"));
        return;
      }
      el?.blur();
    }
  }

  async function ondrop(e: DragEvent) {
    const path = e.dataTransfer?.getData(DATA_MIME);
    if (!path || plain) return;
    e.preventDefault();
    e.stopPropagation();
    // Put the cursor at the drop position.
    const doc = document as Document & { caretRangeFromPoint?: (x: number, y: number) => Range | null };
    const r = doc.caretRangeFromPoint?.(e.clientX, e.clientY);
    if (r && el?.contains(r.startContainer)) {
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(r);
    }
    try {
      const bind = await TemplateAPI.RelativeBind(ref, path);
      insertNode(chip({ bind }));
      focused = false;
      await commit();
    } catch (err) {
      ui.error = message(err);
    }
  }
</script>

<div class="content-editor">
  <div bind:this={el} class="editable" class:plain contenteditable="true" role="textbox" tabindex="0" aria-multiline="true"
    aria-label={t("content.label", { el: n ? label(n.type, n.name) : "" })} spellcheck="false" data-testid="content-editor"
    {oninput} {onkeydown}
    onfocus={() => (focused = true)}
    onblur={async () => {
      focused = false;
      query = null;
      await commit();
      ondone?.();
    }}
    ondragover={(e) => {
      if (!plain && e.dataTransfer?.types.includes(DATA_MIME)) {
        e.preventDefault();
        e.stopPropagation();
      }
    }}
    {ondrop}></div>
  {#if matches.length}
    <ul class="menu" role="listbox" aria-label={t("content.fields")} data-testid="content-menu">
      {#each matches as f, i (f.path)}
        <li role="option" aria-selected={i === index} class:sel={i === index}
          onmousedown={(e) => {
            e.preventDefault();
            pick(f);
          }}>{f.path}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .content-editor {
    position: relative;
  }
  .editable {
    min-height: 1.6em;
    padding: 3px 6px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--surface);
    color: var(--text);
    white-space: pre-wrap;
    line-height: 1.5;
    outline: none;
    cursor: text;
  }
  .editable:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  .editable :global(.chip) {
    display: inline-block;
    padding: 0 5px;
    margin: 0 1px;
    border-radius: 9px;
    background: var(--accent-soft);
    color: var(--accent);
    font-family: var(--font-mono);
    font-size: 0.85em;
    line-height: 1.4;
    user-select: all;
    white-space: nowrap;
  }
  .menu {
    position: absolute;
    z-index: 30;
    left: 0;
    right: 0;
    top: 100%;
    margin: 2px 0 0;
    padding: 2px;
    list-style: none;
    max-height: 200px;
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
  }
  li {
    padding: 2px 6px;
    border-radius: 3px;
    font-family: var(--font-mono);
    font-size: 12px;
    cursor: pointer;
  }
  li.sel {
    background: var(--accent-soft);
  }
</style>
