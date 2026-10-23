<script lang="ts">
  import { TemplateAPI, message, type TableOp, type ViewNode } from "../lib/api";
  import { t } from "../lib/i18n";
  import { node, tpl } from "../lib/template.svelte";
  import { select } from "../lib/selection.svelte";
  import { ui } from "../lib/project.svelte";

  let { n }: { n: ViewNode } = $props();

  // Table, cell and section for the selection.
  const ctx = $derived.by(() => {
    let table: ViewNode | undefined, cell: ViewNode | undefined, section: ViewNode | undefined;
    for (let x: ViewNode | undefined = n; x; x = node(x.parent)) {
      if (x.name === "cell" && !cell) cell = x;
      if (["header", "footer", "rows"].includes(x.name) && !section) section = x;
      if (x.name === "table") {
        table = x;
        break;
      }
    }
    return { table, cell, section };
  });

  async function run(op: Partial<TableOp> & { op: string }) {
    try {
      const res = await TemplateAPI.Table({ ref: "", index: -1, kind: "", count: 0, ...op } as TableOp);
      tpl.history = res.history;
      if (res.select) select(res.select);
      ui.error = "";
    } catch (e) {
      ui.error = message(e);
    }
  }

  const span = $derived(Number(ctx.cell?.attrs.colspan ?? "1"));
</script>

{#if ctx.table}
  <section class="table-tools" data-testid="table-tools">
    <h3>{t("table.title")}</h3>
    <div class="buttons">
      {#if ctx.cell}
        <button onclick={() => run({ op: "addColumn", ref: ctx.cell!.ref })}>{t("table.colAfter")}</button>
        <button onclick={() => run({ op: "removeColumn", ref: ctx.cell!.ref })}>{t("table.colRemove")}</button>
      {:else}
        <button onclick={() => run({ op: "addColumn", ref: ctx.table!.ref })}>{t("table.colAfter")}</button>
      {/if}
      {#if ctx.section}
        <button onclick={() => run({ op: "addRow", ref: (ctx.cell ? node(ctx.cell.parent) : ctx.section)!.ref })}>{t("table.row")}</button>
      {/if}
      {#if !Object.values(tpl.snapshot?.nodes ?? {}).some((x) => x?.parent === ctx.table!.ref && x.name === "header")}
        <button onclick={() => run({ op: "addSection", ref: ctx.table!.ref, kind: "header" })}>{t("table.header")}</button>
      {/if}
      {#if !Object.values(tpl.snapshot?.nodes ?? {}).some((x) => x?.parent === ctx.table!.ref && x.name === "footer")}
        <button onclick={() => run({ op: "addSection", ref: ctx.table!.ref, kind: "footer" })}>{t("table.footer")}</button>
      {/if}
    </div>
    {#if ctx.cell}
      <label class="span">
        {t("table.span")}
        <input type="number" min="1" value={span} onchange={(e) => {
          const count = Number(e.currentTarget.value);
          if (count !== span) run({ op: "span", ref: ctx.cell!.ref, count });
        }} />
      </label>
      {#if ctx.section && ctx.section.name !== "rows"}
        <div class="sums">
          <span>{t("table.sums")}</span>
          <button onclick={() => run({ op: "sum", ref: ctx.cell!.ref, kind: "subtotal" })}>{t("table.subtotal")}</button>
          <button onclick={() => run({ op: "sum", ref: ctx.cell!.ref, kind: "carryOver" })}>{t("table.carryOver")}</button>
          <button onclick={() => run({ op: "sum", ref: ctx.cell!.ref, kind: "total" })}>{t("table.total")}</button>
        </div>
      {/if}
    {/if}
  </section>
{/if}

<style>
  .table-tools {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: var(--space-2);
    margin-bottom: var(--space-2);
    background: var(--surface-2);
  }
  h3 {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    margin: 0 0 var(--space-1);
  }
  .buttons,
  .sums {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .sums {
    margin-top: var(--space-2);
    align-items: center;
  }
  .sums span {
    width: 100%;
    color: var(--text-muted);
    font-size: 12px;
  }
  .span {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
  .span input {
    width: 56px;
  }
  button {
    font-size: 12px;
    padding: 2px 8px;
  }
</style>
