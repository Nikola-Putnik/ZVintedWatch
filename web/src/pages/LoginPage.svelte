<script lang="ts">
  import { api } from '../lib/api';
  import { bootstrap } from '../lib/state.svelte';

  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    try {
      await api.login(password);
      await bootstrap();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="wrap">
  <form class="card login" onsubmit={submit}>
    <img src="/favicon.svg" alt="" width="48" height="48" />
    <h1>ZVintedWatch</h1>
    <p class="text-2">Enter the password of this instance.</p>
    <!-- svelte-ignore a11y_autofocus -->
    <input
      class="input"
      class:invalid={!!error}
      type="password"
      bind:value={password}
      placeholder="Password"
      autocomplete="current-password"
      autofocus
    />
    {#if error}<p class="error small">{error}</p>{/if}
    <button class="btn btn-primary" type="submit" disabled={busy || !password}>
      {#if busy}<span class="spinner"></span>{/if} Log in
    </button>
    <p class="muted small">
      Forgot it? Run <code>zvintedwatch reset-password</code> on the machine running ZVintedWatch.
    </p>
  </form>
</div>

<style>
  .wrap {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 1rem;
  }
  .login {
    width: min(380px, 100%);
    padding: 2rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    align-items: stretch;
    text-align: center;
  }
  .login img {
    align-self: center;
  }
  .error {
    color: var(--danger);
  }
</style>
