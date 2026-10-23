<script lang="ts">
  import Modal from "./Modal.svelte";
  import { PublishAPI, message, type PublishDiff, type PublishResult, type PublishState } from "../lib/api";
  import { t } from "../lib/i18n";

  let { onclose }: { onclose: () => void } = $props();

  let st: PublishState = $state({ url: "", connected: false, id: "" });
  let url = $state("");
  // The key lives only in this field and in the backend storage.
  let key = $state("");
  let id = $state("");
  let diff: PublishDiff | null = $state(null);
  let result: PublishResult | null = $state(null);
  let error = $state("");
  let busy = $state(false);
  let ready = $state(false);

  // Fields are editable only after the defaults have loaded.
  PublishAPI.State().then((s) => {
    st = s;
    url = s.url;
    id = s.id;
    ready = true;
  });

  async function run(f: () => Promise<void>) {
    busy = true;
    error = "";
    try {
      await f();
    } catch (e) {
      error = message(e);
    } finally {
      busy = false;
    }
  }

  const connect = () =>
    run(async () => {
      st = await PublishAPI.Connect(url, key);
      key = "";
      await compare();
    });

  const disconnect = () =>
    run(async () => {
      st = await PublishAPI.Disconnect();
      diff = null;
      result = null;
    });

  async function compare() {
    result = null;
    diff = await PublishAPI.Compare(id);
  }

  const upload = (activate: boolean) =>
    run(async () => {
      result = await PublishAPI.Publish(id, activate);
      diff = await PublishAPI.Compare(id);
    });

  const activate = () =>
    run(async () => {
      if (!result) return;
      await PublishAPI.Activate(id, result.version);
      result = { ...result, active: true };
      diff = await PublishAPI.Compare(id);
    });

  // Only changed lines with two lines of context each.
  const lines = $derived.by(() => {
    const all = diff?.template ?? [];
    const keep = all.map(() => false);
    all.forEach((l, i) => {
      if (l.op !== " ") for (let j = Math.max(0, i - 2); j <= Math.min(all.length - 1, i + 2); j++) keep[j] = true;
    });
    const out: { op: string; text: string; gap?: boolean }[] = [];
    all.forEach((l, i) => {
      if (keep[i]) out.push(l);
      else if (i > 0 && keep[i - 1]) out.push({ op: " ", text: "…", gap: true });
    });
    return out;
  });
</script>

<Modal title={t("publish.title")}>
  <div class="publish" data-testid="publish" data-ready={ready}>
    {#if !st.connected}
      <label>{t("publish.url")}<input data-testid="publish-url" disabled={!ready} bind:value={url} placeholder="https://pdx.example.org" /></label>
      <label>{t("publish.key")}<input data-testid="publish-key" disabled={!ready} type="password" autocomplete="off" bind:value={key} /></label>
      <p class="hint">{t("publish.keyHint")}</p>
      <label>{t("publish.id")}<input data-testid="publish-id" disabled={!ready} bind:value={id} /></label>
    {:else}
      <p class="ok" data-testid="publish-connected">{t("publish.connected", { url: st.url })}</p>
      <label>{t("publish.id")}<input data-testid="publish-id" bind:value={id} onchange={() => run(compare)} /></label>
      <p class="hint">{t("publish.idHint")} {t("publish.saveHint")}</p>
      {#if diff}
        <div class="diff" data-testid="publish-diff">
          {#if !diff.active}
            <p>{t("publish.new")}</p>
          {:else}
            <p>{t("publish.active", { v: diff.active, n: diff.versions })}</p>
          {/if}
          {#if diff.same}
            <p data-testid="publish-same">{t("publish.same")}</p>
          {:else}
            <h3>{t("publish.files")}</h3>
            <ul>
              {#each diff.files ?? [] as f (f.name)}
                <li data-file={f.name}><code>{f.name}</code> <span class="kind {f.kind}">{t(`publish.${f.kind}` as "publish.added")}</span></li>
              {/each}
            </ul>
            {#if diff.active && lines.length}
              <h3>{t("publish.template")}</h3>
              <pre data-testid="publish-lines">{#each lines as l, i (i)}<span class:add={l.op === "+"} class:del={l.op === "-"} class:gap={l.gap}>{l.gap ? "" : l.op} {l.text}
</span>{/each}</pre>
            {/if}
          {/if}
        </div>
      {/if}
      {#if result}
        <p class="ok" data-testid="publish-result">
          {result.created ? t("publish.created", { v: result.version }) : t("publish.existing", { v: result.version })}
          {#if result.active}{t("publish.isActive", { v: result.version })}{/if}
        </p>
      {/if}
    {/if}
    {#if error}<pre class="error" role="alert" data-testid="publish-error">{error}</pre>{/if}
  </div>
  {#snippet actions()}
    {#if !st.connected}
      <button onclick={onclose}>{t("publish.close")}</button>
      <button class="primary" data-testid="publish-connect" disabled={busy || !url || !key} onclick={connect}>{t("publish.connect")}</button>
    {:else}
      <button onclick={disconnect} disabled={busy}>{t("publish.disconnect")}</button>
      <button onclick={onclose}>{t("publish.close")}</button>
      {#if result && !result.active}
        <button class="primary" data-testid="publish-activate" disabled={busy} onclick={activate}>{t("publish.activate", { v: result.version })}</button>
      {:else}
        <button data-testid="publish-upload" disabled={busy || !diff || diff.same} onclick={() => upload(false)}>{t("publish.upload")}</button>
        <button class="primary" data-testid="publish-upload-activate" disabled={busy || !diff || diff.same} onclick={() => upload(true)}>{t("publish.uploadActivate")}</button>
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .publish {
    width: min(520px, 80vw);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 12px;
  }
  .hint {
    color: var(--text-muted);
    font-size: 12px;
    margin: 0;
  }
  .ok {
    margin: 0;
  }
  .diff {
    max-height: 45vh;
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
  .kind {
    font-size: 11px;
    color: var(--text-muted);
  }
  pre {
    font-size: 11px;
    margin: 0;
    white-space: pre-wrap;
  }
  .add {
    color: var(--success, #1a7f37);
  }
  .del {
    color: var(--danger, #cf222e);
  }
  .gap {
    color: var(--text-muted);
  }
  .error {
    color: var(--danger, #cf222e);
  }
</style>
