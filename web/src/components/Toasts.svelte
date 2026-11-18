<script lang="ts">
  import { toasts, dismissToast } from '../lib/state.svelte';
  import Icon from './Icon.svelte';
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}">
      <Icon name={t.kind === 'error' ? 'alert' : t.kind === 'info' ? 'info' : 'check'} size={17} />
      <span class="grow">{t.text}</span>
      <button aria-label="Dismiss" onclick={() => dismissToast(t.id)}><Icon name="x" size={14} /></button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    bottom: 1.25rem;
    right: 1.25rem;
    z-index: 80;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    width: min(380px, calc(100vw - 2.5rem));
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
    padding: 0.75rem 0.85rem;
    border-radius: var(--radius);
    background: var(--surface);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-lg);
    animation: pop 0.18s ease-out;
    font-size: 0.93em;
  }
  .toast.success :global(.icon) {
    color: var(--success);
  }
  .toast.error {
    border-color: var(--danger);
  }
  .toast.error :global(.icon:first-child) {
    color: var(--danger);
  }
  .toast.info :global(.icon:first-child) {
    color: var(--accent);
  }
  button {
    border: none;
    background: none;
    color: var(--text-3);
    cursor: pointer;
    padding: 2px;
  }
  @keyframes pop {
    from {
      transform: translateY(8px);
      opacity: 0;
    }
  }
</style>
