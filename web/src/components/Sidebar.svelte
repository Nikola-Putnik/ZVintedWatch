<script lang="ts">
  import { app } from '../lib/state.svelte';
  import { isActive } from '../lib/router.svelte';
  import { activity } from '../lib/activity.svelte';
  import { applyTheme, effectiveTheme, getTheme, type Theme } from '../lib/theme';
  import Icon from './Icon.svelte';

  let { open = false }: { open?: boolean } = $props();

  let theme = $state<Theme>(getTheme());
  function toggleTheme() {
    theme = effectiveTheme(theme) === 'dark' ? 'light' : 'dark';
    applyTheme(theme);
  }

  const statusById = $derived(Object.fromEntries((app.status?.providers ?? []).map((p) => [p.id, p])));
  const telegram = $derived(app.status?.telegram);
  // Release versions are numbers ("1.2.0"); development builds are "dev".
  const versionLabel = $derived.by(() => {
    const v = app.meta?.version ?? '';
    return /^\d/.test(v) ? 'v' + v : v;
  });
</script>

<nav class="sidebar" class:open aria-label="Main navigation">
  <a class="logo" href="#/">
    <img src="/favicon.svg" alt="" width="30" height="30" />
    <span>ZVintedWatch</span>
  </a>

  <div class="group">
    <a class="item" class:active={isActive('/')} href="#/"><Icon name="home" /> Dashboard</a>
  </div>

  <div class="group">
    <div class="group-title">Sites</div>
    {#each app.meta?.providers ?? [] as p (p.id)}
      {@const st = statusById[p.id]}
      <a class="item" class:active={isActive('/site/' + p.id)} class:off={st && !st.enabled} href="#/site/{p.id}">
        <span class="site-dot" style="background: {p.color}"></span>
        <span class="grow">{p.name}</span>
        {#if st && !st.enabled}
          <span class="pill" title="Site paused">off</span>
        {:else if st?.health.state === 'cooldown'}
          <span class="pill danger" title="The site refused requests; cooling down">cooldown</span>
        {:else if st}
          <span class="pill" title="Active alerts">{st.enabledAlerts}</span>
        {/if}
      </a>
    {/each}
    <div class="coming muted small">More sites can be added as modules.</div>
  </div>

  <div class="group">
    <div class="group-title">Monitor</div>
    <a class="item" class:active={isActive('/activity')} href="#/activity">
      <Icon name="terminal" />
      <span class="grow">Live activity</span>
      <span class="live" class:on={activity.connected} title={activity.connected ? 'Connected' : 'Disconnected'}></span>
    </a>
    <a class="item" class:active={isActive('/settings')} href="#/settings"><Icon name="settings" /> Settings</a>
  </div>

  <div class="footer">
    <a class="telegram" href="#/settings" class:warn={!telegram?.configured}>
      <Icon name="send" size={16} />
      {#if telegram?.configured}
        <span class="ellipsis">@{telegram.botUsername}</span>
      {:else}
        <span>Telegram not set up</span>
      {/if}
    </a>
    <div class="row">
      <span class="muted small grow">{versionLabel}</span>
      <button class="btn btn-ghost btn-icon btn-sm" onclick={toggleTheme} aria-label="Toggle dark mode" title="Toggle dark mode">
        <Icon name={effectiveTheme(theme) === 'dark' ? 'sun' : 'moon'} size={16} />
      </button>
    </div>
  </div>
</nav>

<style>
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: var(--sidebar-w);
    background: var(--surface);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    padding: 1rem 0.75rem;
    gap: 1.1rem;
    z-index: 30;
    overflow-y: auto;
  }
  .logo {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.25rem 0.5rem 0.5rem;
    font-weight: 750;
    font-size: 1.08rem;
    letter-spacing: -0.02em;
    color: var(--text);
    text-decoration: none !important;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .group-title {
    font-size: 0.72rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    padding: 0 0.6rem 0.35rem;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 0.5rem 0.6rem;
    border-radius: 8px;
    color: var(--text-2);
    font-weight: 550;
    text-decoration: none !important;
  }
  .item:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .item.active {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .item.off .site-dot {
    opacity: 0.35;
  }
  .site-dot {
    width: 12px;
    height: 12px;
    border-radius: 4px;
    margin: 0 3px;
  }
  .pill {
    font-size: 0.72rem;
    font-weight: 700;
    padding: 0 0.45rem;
    border-radius: 999px;
    background: var(--surface-2);
    color: var(--text-2);
  }
  .pill.danger {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .coming {
    padding: 0.25rem 0.6rem;
  }
  .live {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-3);
  }
  .live.on {
    background: var(--success);
    box-shadow: 0 0 0 3px var(--success-soft);
  }
  .footer {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0 0.25rem;
  }
  .telegram {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.55rem 0.65rem;
    border-radius: 8px;
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 0.88em;
    font-weight: 550;
    text-decoration: none !important;
  }
  .telegram.warn {
    background: var(--warning-soft);
    color: var(--warning);
  }
  @media (max-width: 960px) {
    .sidebar {
      transform: translateX(-100%);
      transition: transform 0.2s ease;
      box-shadow: var(--shadow-lg);
    }
    .sidebar.open {
      transform: none;
    }
  }
</style>
