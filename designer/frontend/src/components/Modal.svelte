<script lang="ts">
  import type { Snippet } from "svelte";
  let { title, children, actions }: { title: string; children: Snippet; actions: Snippet } = $props();
</script>

<div class="backdrop">
  <div class="modal" role="dialog" aria-modal="true" aria-label={title}>
    <h1>{title}</h1>
    <div class="body">{@render children()}</div>
    <div class="actions">{@render actions()}</div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.3);
    display: grid;
    place-items: center;
    z-index: 100;
  }
  .modal {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.25);
    padding: var(--space-4);
    min-width: 380px;
    max-width: 560px;
  }
  h1 {
    font-size: 15px;
    margin: 0 0 var(--space-3);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }
</style>
