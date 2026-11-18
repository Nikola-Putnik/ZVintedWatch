<script lang="ts">
  import { onMount } from 'svelte';
  import { app, bootstrap, startStatusPolling, stopStatusPolling } from './lib/state.svelte';
  import { route } from './lib/router.svelte';
  import { startActivity, stopActivity } from './lib/activity.svelte';
  import Sidebar from './components/Sidebar.svelte';
  import Toasts from './components/Toasts.svelte';
  import Confirm from './components/Confirm.svelte';
  import Icon from './components/Icon.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import SitePage from './pages/SitePage.svelte';
  import ActivityPage from './pages/ActivityPage.svelte';
  import SettingsPage from './pages/SettingsPage.svelte';
  import LoginPage from './pages/LoginPage.svelte';

  let menuOpen = $state(false);

  onMount(() => {
    bootstrap();
    return () => {
      stopStatusPolling();
      stopActivity();
    };
  });

  // Start live data once the user is authenticated.
  $effect(() => {
    if (app.ready && !app.needsLogin && app.meta) {
      startStatusPolling();
      startActivity();
    } else {
      stopStatusPolling();
      stopActivity();
    }
  });

  // Close the mobile menu after navigating.
  $effect(() => {
    route.path;
    menuOpen = false;
  });

  const page = $derived(route.parts[0] ?? '');
</script>

{#if !app.ready}
  <div class="splash"><span class="spinner"></span></div>
{:else if app.needsLogin}
  <LoginPage />
{:else if app.bootError}
  <div class="splash">
    <div class="card card-pad boot-error">
      <h2>ZVintedWatch is not reachable</h2>
      <p class="text-2">{app.bootError}</p>
      <button class="btn btn-primary" onclick={bootstrap}><Icon name="refresh" size={16} /> Retry</button>
    </div>
  </div>
{:else}
  <div class="layout">
    <header class="topbar">
      <button class="btn btn-ghost btn-icon" aria-label="Menu" onclick={() => (menuOpen = !menuOpen)}>
        <Icon name="menu" />
      </button>
      <span class="brand">ZVintedWatch</span>
    </header>

    <Sidebar open={menuOpen} />
    {#if menuOpen}
      <div class="scrim" role="presentation" onclick={() => (menuOpen = false)}></div>
    {/if}

    <main>
      {#key route.path.split('/').slice(0, 3).join('/')}
        {#if page === ''}
          <Dashboard />
        {:else if page === 'site' && route.parts[1]}
          <SitePage providerId={route.parts[1]} tab={route.parts[2] ?? 'alerts'} />
        {:else if page === 'activity'}
          <ActivityPage />
        {:else if page === 'settings'}
          <SettingsPage />
        {:else}
          <div class="page">
            <h1>Page not found</h1>
            <p class="text-2" style="margin-top: .5rem"><a href="#/">Back to the dashboard</a></p>
          </div>
        {/if}
      {/key}
    </main>
  </div>
{/if}

<Toasts />
<Confirm />

<style>
  .splash {
    min-height: 100vh;
    display: grid;
    place-items: center;
    color: var(--text-3);
    padding: 1rem;
  }
  .boot-error {
    max-width: 420px;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    align-items: flex-start;
  }
  .layout {
    min-height: 100vh;
  }
  main {
    margin-left: var(--sidebar-w);
    min-height: 100vh;
  }
  .topbar {
    display: none;
  }
  .scrim {
    display: none;
  }
  @media (max-width: 960px) {
    main {
      margin-left: 0;
    }
    .topbar {
      display: flex;
      align-items: center;
      gap: 0.5rem;
      position: sticky;
      top: 0;
      z-index: 20;
      padding: 0.5rem 0.75rem;
      background: var(--surface);
      border-bottom: 1px solid var(--border);
    }
    .brand {
      font-weight: 700;
    }
    .scrim {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 29;
      background: rgba(10, 12, 25, 0.4);
    }
  }
</style>
