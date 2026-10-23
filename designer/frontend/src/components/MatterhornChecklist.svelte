<script lang="ts">
  // Manual checkpoints according to Matterhorn 1.1. The state of the checks applies per
  // template and stays stored on this machine.
  import { checks } from "../lib/matterhorn";
  import { t } from "../lib/i18n";
  import { project } from "../lib/project.svelte";

  let open = $state(false);
  let done = $state<Record<string, boolean>>({});
  const key = $derived(`pdx.matterhorn.${project.path || project.name}`);

  $effect(() => {
    try {
      done = JSON.parse(localStorage.getItem(key) ?? "{}");
    } catch {
      done = {};
    }
  });

  function toggle(id: string, v: boolean) {
    done = { ...done, [id]: v };
    try {
      localStorage.setItem(key, JSON.stringify(done));
    } catch {
      // without storage the state applies only until closing
    }
  }

  const relevant = $derived(checks.filter((c) => !c.na));
  const count = $derived(relevant.filter((c) => done[c.id]).length);
</script>

<section class="matterhorn" data-testid="matterhorn">
  <button class="head" aria-expanded={open} onclick={() => (open = !open)}>
    <span aria-hidden="true">{open ? "▾" : "▸"}</span>
    {t("matterhorn.title")}
    <span class="progress" data-testid="matterhorn-progress">{t("matterhorn.progress", { n: count, total: relevant.length })}</span>
  </button>
  {#if open}
    <p class="hint">{t("matterhorn.hint")}</p>
    <ul>
      {#each checks as c (c.id)}
        <li class:na={!!c.na}>
          <label>
            <input type="checkbox" disabled={!!c.na} checked={!c.na && !!done[c.id]} onchange={(e) => toggle(c.id, e.currentTarget.checked)} data-check={c.id} />
            <code>{c.id}</code>
            <span>{c.text}{#if c.na}{" "}<em>{t("matterhorn.na", { reason: c.na })}</em>{/if}</span>
          </label>
        </li>
      {/each}
    </ul>
    <p class="source">{t("matterhorn.source")}</p>
  {/if}
</section>

<style>
  .matterhorn {
    margin-top: var(--space-3);
  }
  .head {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    border: 0;
    background: none;
    padding: 2px 0;
    color: var(--text);
    font-weight: 600;
  }
  .progress {
    font-weight: normal;
    color: var(--text-muted);
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  label {
    display: grid;
    grid-template-columns: auto 56px 1fr;
    gap: var(--space-2);
    align-items: baseline;
    padding: 1px 0;
  }
  code {
    color: var(--text-muted);
    font-size: 11px;
  }
  .na span {
    color: var(--text-muted);
  }
  em {
    font-size: 11px;
  }
  .hint,
  .source {
    color: var(--text-muted);
    font-size: 11px;
    margin: var(--space-1) 0;
  }
</style>
