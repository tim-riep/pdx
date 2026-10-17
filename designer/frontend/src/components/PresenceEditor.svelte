<script lang="ts">
  import { TemplateAPI, type DataField, type ViewNode } from "../lib/api";
  import { propLabel, t } from "../lib/i18n";
  import { apply, tpl } from "../lib/template.svelte";
  import XPathInput from "./XPathInput.svelte";

  let { n }: { n: ViewNode } = $props();

  type Mode = "always" | "exists" | "filled" | "custom";
  const value = $derived(n.attrs.presence ?? "");

  // Die häufigen Fälle erkennt die Auswahl am Ausdruck wieder.
  const parsed = $derived.by((): { mode: Mode; field: string } => {
    if (!value) return { mode: "always", field: "" };
    let m = /^exists\(([^()]+)\)$/.exec(value);
    if (m) return { mode: "exists", field: m[1] };
    m = /^normalize-space\(([^()]+)\)$/.exec(value);
    if (m) return { mode: "filled", field: m[1] };
    return { mode: "custom", field: "" };
  });

  // Gewählt, aber noch ohne Ausdruck: eigener Ausdruck vor dem Tippen.
  let pending: Mode | null = $state(null);
  const mode = $derived(pending ?? parsed.mode);

  let fields: DataField[] = $state.raw([]);
  $effect(() => {
    const ref = n.ref;
    void tpl.snapshot?.rev;
    TemplateAPI.XPathFields(ref)
      .then((f) => (fields = f))
      .catch(() => (fields = []));
  });
  // Optionale Knoten zuerst, sie sind der übliche Grund für eine Bedingung.
  const sorted = $derived([...fields].sort((a, b) => Number(b.optional) - Number(a.optional)));

  function expr(m: Mode, field: string): string {
    if (m === "exists") return `exists(${field})`;
    if (m === "filled") return `normalize-space(${field})`;
    return "";
  }

  async function set(v: string) {
    if (v === value) return;
    await apply(t("cmd.setAttr", { prop: propLabel("presence") }), { op: "setAttr", ref: n.ref, name: "presence", value: v });
  }

  function choose(m: Mode) {
    pending = null;
    if (m === "custom") {
      pending = "custom";
      return;
    }
    if (m === "always") return set("");
    const field = parsed.field || sorted[0]?.path || "";
    if (field) set(expr(m, field));
    else pending = m;
  }

  $effect(() => {
    // Nach einer Änderung im Modell gilt wieder der Ausdruck.
    void value;
    pending = null;
  });
</script>

<div class="presence" data-testid="presence">
  <div class="modes" role="radiogroup" aria-label={propLabel("presence")}>
    {#each ["always", "exists", "filled", "custom"] as m (m)}
      <label>
        <input type="radio" name={"presence-" + n.ref} checked={mode === m} onchange={() => choose(m as Mode)} />
        {t(`presence.${m}`)}
      </label>
    {/each}
  </div>
  {#if mode === "exists" || mode === "filled"}
    <select aria-label={t("presence.field")} value={parsed.field} onchange={(e) => set(expr(mode, e.currentTarget.value))}>
      {#if !parsed.field}<option value=""></option>{/if}
      {#each sorted as f (f.path)}
        <option value={f.path}>{f.path}{f.optional ? ` (${t("data.optional")})` : ""}</option>
      {/each}
    </select>
  {/if}
  {#if mode !== "always"}
    <XPathInput ref={n.ref} prop="presence" {value} label={propLabel("presence")} />
  {/if}
</div>

<style>
  .presence {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .modes {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .modes label {
    display: flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
  }
  select {
    width: 100%;
    box-sizing: border-box;
  }
</style>
