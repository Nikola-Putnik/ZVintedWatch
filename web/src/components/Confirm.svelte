<script lang="ts" module>
  // Imperative confirmation dialog: `if (await confirmDialog({...})) { ... }`
  interface Request {
    title: string;
    message: string;
    confirmLabel?: string;
    danger?: boolean;
    resolve: (ok: boolean) => void;
  }
  const state = $state({ current: null as Request | null });

  export function confirmDialog(opts: Omit<Request, 'resolve'>): Promise<boolean> {
    return new Promise((resolve) => {
      state.current = { ...opts, resolve };
    });
  }
</script>

<script lang="ts">
  function answer(ok: boolean) {
    state.current?.resolve(ok);
    state.current = null;
  }
  function onkeydown(e: KeyboardEvent) {
    if (!state.current) return;
    if (e.key === 'Escape') answer(false);
    if (e.key === 'Enter') answer(true);
  }
</script>

<svelte:window {onkeydown} />

{#if state.current}
  <div class="overlay" role="presentation" onclick={() => answer(false)}></div>
  <div class="dialog card" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title">
    <h2 id="confirm-title">{state.current.title}</h2>
    <p class="text-2">{state.current.message}</p>
    <div class="actions">
      <button class="btn" onclick={() => answer(false)}>Cancel</button>
      <button class="btn {state.current.danger ? 'btn-danger' : 'btn-primary'}" onclick={() => answer(true)}>
        {state.current.confirmLabel ?? 'Confirm'}
      </button>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(10, 12, 25, 0.45);
    z-index: 60;
  }
  .dialog {
    position: fixed;
    left: 50%;
    top: 30%;
    transform: translate(-50%, -30%);
    width: min(440px, calc(100vw - 2rem));
    padding: 1.4rem;
    z-index: 61;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    box-shadow: var(--shadow-lg);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
    margin-top: 0.4rem;
  }
</style>
