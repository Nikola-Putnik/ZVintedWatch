<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { app, providerInfo, refreshStatus, setSiteEnabled, toast, toastError } from '../lib/state.svelte';
  import type { Alert, AlertView, Destination, Match, SearchStatus } from '../lib/types';
  import { formatInterval, plural } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import Toggle from '../components/Toggle.svelte';
  import HealthBadge from '../components/HealthBadge.svelte';
  import RelativeTime from '../components/RelativeTime.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import MatchCard from '../components/MatchCard.svelte';
  import AlertEditor from '../components/AlertEditor.svelte';
  import { confirmDialog } from '../components/Confirm.svelte';

  let { providerId, tab }: { providerId: string; tab: string } = $props();

  const info = $derived(providerInfo(providerId));
  const status = $derived(app.status?.providers.find((p) => p.id === providerId));
  const siteEnabled = $derived(status?.enabled ?? true);

  let alerts = $state<AlertView[]>([]);
  let searches = $state<SearchStatus[]>([]);
  let matches = $state<Match[]>([]);
  let destinations = $state<Destination[]>([]);
  let loaded = $state(false);
  let filter = $state('');
  let editing = $state<Alert | null>(null);

  async function loadAlerts() {
    [alerts, searches] = await Promise.all([api.alerts(providerId), api.searches(providerId)]);
  }
  async function loadAll() {
    try {
      const [a, s, d] = await Promise.all([api.alerts(providerId), api.searches(providerId), api.destinations()]);
      alerts = a;
      searches = s;
      destinations = d;
      if (tab === 'matches') matches = await api.matches({ provider: providerId, limit: 120 });
    } catch (e) {
      toastError(e);
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    loadAll();
    const timer = setInterval(async () => {
      try {
        searches = await api.searches(providerId);
        if (tab === 'matches') matches = await api.matches({ provider: providerId, limit: 120 });
      } catch {
        /* keep the previous data */
      }
    }, 5000);
    return () => clearInterval(timer);
  });

  const searchByKey = $derived(Object.fromEntries(searches.map((s) => [s.key, s])));
  const destName = $derived(Object.fromEntries(destinations.map((d) => [d.id, d.name])));

  const visible = $derived(
    alerts.filter((a) => {
      const q = filter.trim().toLowerCase();
      return !q || a.name.toLowerCase().includes(q) || a.searchInput.toLowerCase().includes(q);
    }),
  );

  // Enabled alerts grouped by the search that serves them.
  const groups = $derived.by(() => {
    const map = new Map<string, AlertView[]>();
    for (const a of visible) {
      if (!a.enabled || a.searchError) continue;
      const list = map.get(a.searchKey) ?? [];
      list.push(a);
      map.set(a.searchKey, list);
    }
    return [...map.entries()].map(([key, list]) => ({ key, alerts: list, search: searchByKey[key], label: list[0].searchLabel, url: list[0].searchUrl }));
  });
  const paused = $derived(visible.filter((a) => !a.enabled && !a.searchError));
  const broken = $derived(visible.filter((a) => a.searchError));

  function newAlert(): Alert {
    const rec = info?.recommendedIntervalSec ?? 60;
    return {
      id: 0,
      provider: providerId,
      name: '',
      enabled: true,
      searchInput: '',
      domain: info?.defaultDomain ?? '',
      title: { include: [], includeMode: 'all', exclude: [] },
      description: { include: [], includeMode: 'all', exclude: [] },
      priceMin: null,
      priceMax: null,
      intervalMinSec: rec,
      intervalMaxSec: rec,
      destinationIds: destinations.map((d) => d.id),
    };
  }

  function strip(a: AlertView): Alert {
    const { searchLabel, searchKey, searchUrl, searchId, searchError, sharedWith, ...rest } = a;
    return structuredClone($state.snapshot(rest)) as Alert;
  }

  function edit(a: AlertView) {
    editing = strip(a);
  }
  function duplicate(a: AlertView) {
    editing = { ...strip(a), id: 0, name: a.name + ' (copy)' };
  }
  function addToSearch(a: AlertView) {
    editing = { ...newAlert(), searchInput: a.searchInput, domain: a.domain };
  }

  async function toggle(a: AlertView, enabled: boolean) {
    try {
      await api.setAlertEnabled(a.id, enabled);
      toast(enabled ? `“${a.name}” enabled` : `“${a.name}” paused`, 'info');
      await loadAlerts();
      refreshStatus();
    } catch (e) {
      toastError(e);
      await loadAlerts();
    }
  }

  async function remove(a: AlertView) {
    const ok = await confirmDialog({
      title: 'Delete this alert?',
      message: `“${a.name}” will be deleted. Its past matches stay in the history.`,
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    try {
      await api.deleteAlert(a.id);
      toast(`“${a.name}” deleted`);
      await loadAlerts();
      refreshStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function refreshNow(s: SearchStatus) {
    try {
      await api.refreshSearch(s.id);
      toast('Refresh scheduled', 'info');
      setTimeout(async () => (searches = await api.searches(providerId)), 1500);
    } catch (e) {
      toastError(e);
    }
  }

  async function resetCooldown() {
    try {
      await api.resetCooldown(providerId);
      toast('Requests resumed', 'info');
      refreshStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function saved() {
    editing = null;
    await loadAlerts();
    refreshStatus();
  }

  function priceLabel(a: Alert, currency = '€'): string {
    if (a.priceMin != null && a.priceMax != null) return `${a.priceMin}–${a.priceMax} ${currency}`;
    if (a.priceMax != null) return `≤ ${a.priceMax} ${currency}`;
    if (a.priceMin != null) return `≥ ${a.priceMin} ${currency}`;
    return '';
  }
</script>

{#if !info}
  <div class="page"><h1>Unknown site</h1></div>
{:else}
  <div class="page" style="--site: {info.color}">
    <div class="page-header">
      <div class="row">
        <span class="site-mark">{info.name.slice(0, 1)}</span>
        <div>
          <div class="row wrap">
            <h1>{info.name}</h1>
            {#if siteEnabled}
              <HealthBadge health={status?.health} />
            {:else}
              <span class="badge badge-warning"><Icon name="pause" size={11} /> Paused</span>
            {/if}
          </div>
          <p>
            {plural(status?.enabledAlerts ?? 0, 'active alert')} · {plural(status?.searches ?? 0, 'search', 'searches')} ·
            ~{Math.round(status?.requestsPerHour ?? 0)} requests/hour · {status?.health.requestsLastHour ?? 0} sent in the last hour
          </p>
        </div>
      </div>
      <div class="row header-actions">
        <label class="row site-switch" title={siteEnabled ? `Pause every ${info.name} alert` : `Resume ${info.name}`}>
          <Toggle checked={siteEnabled} label={siteEnabled ? 'Pause site' : 'Resume site'}
            onchange={(v) => setSiteEnabled(providerId, v)} />
          <span class="small text-2">{siteEnabled ? 'Site active' : 'Site paused'}</span>
        </label>
        <button class="btn btn-primary" onclick={() => (editing = newAlert())}><Icon name="plus" size={16} /> New alert</button>
      </div>
    </div>

    {#if !siteEnabled}
      <div class="callout warning cooldown">
        <Icon name="pause" size={18} />
        <div class="grow">
          <strong>{info.name} is paused.</strong> Its alerts are kept but no request is sent to {info.name}.
          Other sites are not affected. When you resume it, listings posted in the meantime are not sent.
        </div>
        <button class="btn btn-sm" onclick={() => setSiteEnabled(providerId, true)}>Resume</button>
      </div>
    {:else if status?.health.state === 'cooldown' || status?.health.state === 'degraded'}
      <div class="callout danger cooldown">
        <Icon name="alert" size={18} />
        <div class="grow">
          <strong>{info.name} refused some requests</strong> ({status.health.reason}). ZVintedWatch pauses automatically and resumes
          <RelativeTime time={status.health.cooldownUntil} />. Consider longer intervals or a proxy (Settings → Network).
        </div>
        <button class="btn btn-sm" onclick={resetCooldown}>Resume now</button>
      </div>
    {/if}

    <nav class="tabs">
      <a class="tab" class:active={tab === 'alerts'} href="#/site/{providerId}">Alerts <span class="count">{alerts.length}</span></a>
      <a class="tab" class:active={tab === 'searches'} href="#/site/{providerId}/searches">Searches <span class="count">{searches.length}</span></a>
      <a class="tab" class:active={tab === 'matches'} href="#/site/{providerId}/matches">Matches</a>
    </nav>

    {#if tab === 'alerts'}
      {#if loaded && alerts.length === 0}
        <EmptyState icon="bell" title="No alert yet"
          text="An alert watches a {info.name} search and tells you on Telegram when a new listing matches your keywords.">
          <button class="btn btn-primary" onclick={() => (editing = newAlert())}><Icon name="plus" size={16} /> Create an alert</button>
        </EmptyState>
      {:else}
        <div class="toolbar">
          <div class="filter">
            <Icon name="search" size={15} />
            <input bind:value={filter} placeholder="Filter alerts" aria-label="Filter alerts" />
          </div>
          <span class="muted small">Alerts on the same search share a single request.</span>
        </div>

        {#each groups as g (g.key)}
          <section class="group card">
            <header>
              <Icon name="layers" size={16} />
              <div class="grow">
                <div class="row wrap">
                  <strong class="ellipsis">{g.label}</strong>
                  <a href={g.url} target="_blank" rel="noopener noreferrer" title="Open on {info.name}"><Icon name="external" size={13} /></a>
                  {#if g.alerts.length > 1}<span class="badge badge-success">1 request → {g.alerts.length} alerts</span>{/if}
                </div>
                {#if g.search}
                  <div class="muted small row wrap search-meta">
                    <span><Icon name="clock" size={12} /> {formatInterval(g.search.minIntervalSec, g.search.maxIntervalSec)}</span>
                    <span>
                      {#if g.search.running}refreshing…{:else}next <RelativeTime time={g.search.nextRun} />{/if}
                    </span>
                    <span>last <RelativeTime time={g.search.lastPolledAt} /></span>
                    {#if g.search.lastStatus === 'error' || g.search.lastStatus === 'cooldown'}
                      <span class="err ellipsis" title={g.search.lastError}>⚠ {g.search.lastError}</span>
                    {:else if !g.search.seeded}
                      <span>recording baseline…</span>
                    {/if}
                  </div>
                {/if}
              </div>
              <button class="btn btn-sm btn-ghost" title="Add another alert on this search" onclick={() => addToSearch(g.alerts[0])}>
                <Icon name="plus" size={14} /> Alert
              </button>
              {#if g.search}
                <button class="btn btn-sm btn-ghost btn-icon" title="Refresh now" onclick={() => refreshNow(g.search)}><Icon name="refresh" size={15} /></button>
              {/if}
            </header>
            {#each g.alerts as a (a.id)}
              {@render alertRow(a)}
            {/each}
          </section>
        {/each}

        {#if broken.length}
          <h3 class="sub">Needs attention</h3>
          <section class="group card">
            {#each broken as a (a.id)}
              {@render alertRow(a)}
            {/each}
          </section>
        {/if}

        {#if paused.length}
          <h3 class="sub">Paused</h3>
          <section class="group card paused">
            {#each paused as a (a.id)}
              {@render alertRow(a)}
            {/each}
          </section>
        {/if}
      {/if}
    {:else if tab === 'searches'}
      {#if searches.length === 0}
        <EmptyState icon="search" title="No active search" text="Searches are created automatically from your enabled alerts." />
      {:else}
        <div class="card table-wrap">
          <table class="table">
            <thead>
              <tr><th>Search</th><th>Alerts</th><th>Interval</th><th>Last refresh</th><th>Next</th><th>Results</th><th></th></tr>
            </thead>
            <tbody>
              {#each searches as s (s.key)}
                <tr>
                  <td>
                    <div class="row"><strong class="ellipsis">{s.label}</strong>
                      <a href={s.webUrl} target="_blank" rel="noopener noreferrer"><Icon name="external" size={13} /></a></div>
                    {#if s.lastError}<div class="err small ellipsis" title={s.lastError}>{s.lastError}</div>{/if}
                  </td>
                  <td class="small">{s.alerts.map((a) => a.name).join(', ')}</td>
                  <td class="small nowrap">{formatInterval(s.minIntervalSec, s.maxIntervalSec)}<div class="muted">~{Math.round(s.requestsPerHour)}/h</div></td>
                  <td class="small nowrap"><RelativeTime time={s.lastPolledAt} /></td>
                  <td class="small nowrap">{#if s.running}<span class="spinner"></span>{:else}<RelativeTime time={s.nextRun} />{/if}</td>
                  <td class="small nowrap">{s.lastResults} listings{#if s.lastNew}<div class="new">+{s.lastNew} new</div>{/if}</td>
                  <td><button class="btn btn-sm" onclick={() => refreshNow(s)}><Icon name="refresh" size={14} /> Refresh</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {:else}
      {#if loaded && matches.length === 0}
        <EmptyState icon="tag" title="No match yet" text="Listings matching your {info.name} alerts will be shown here." />
      {:else}
        <div class="matches">
          {#each matches as m (m.id)}
            <MatchCard match={m} />
          {/each}
        </div>
      {/if}
    {/if}
  </div>

  {#if editing}
    <AlertEditor provider={info} alert={editing} {destinations} onclose={() => (editing = null)} onsaved={saved} />
  {/if}
{/if}

{#snippet alertRow(a: AlertView)}
  <div class="alert-row" class:disabled={!a.enabled}>
    <Toggle checked={a.enabled} label={a.enabled ? 'Pause' : 'Enable'} onchange={(v) => toggle(a, v)} />
    <button class="alert-main" onclick={() => edit(a)}>
      <div class="name">{a.name}</div>
      <div class="rules">
        {#if a.searchError}
          <span class="err small">{a.searchError}</span>
        {/if}
        {#if !a.enabled && a.searchLabel}<span class="chip">{a.searchLabel}</span>{/if}
        {#each a.title.include as t}<span class="chip chip-include" title="Title must contain">{t}</span>{/each}
        {#if a.title.include.length > 1 && a.title.includeMode === 'any'}<span class="muted small">(any)</span>{/if}
        {#each a.title.exclude as t}<span class="chip chip-exclude" title="Title must not contain">{t}</span>{/each}
        {#if a.description.include.length || a.description.exclude.length}
          <span class="chip" title="Description rules: {[...a.description.include, ...a.description.exclude.map((x) => 'not ' + x)].join(', ')}">
            + description rules
          </span>
        {/if}
        {#if priceLabel(a)}<span class="chip"><Icon name="tag" size={11} /> {priceLabel(a)}</span>{/if}
        {#if !a.title.include.length && !a.title.exclude.length && !a.description.include.length && !a.description.exclude.length && !priceLabel(a)}
          <span class="muted small">Every new listing</span>
        {/if}
      </div>
    </button>
    <div class="side small muted">
      <span title="Refresh interval"><Icon name="clock" size={12} /> {formatInterval(a.intervalMinSec, a.intervalMaxSec).replace('every ', '')}</span>
      <span title={a.destinationIds.map((id) => destName[id]).filter(Boolean).join(', ') || 'No Telegram chat'}>
        <Icon name="send" size={12} /> {a.destinationIds.filter((id) => destName[id]).length || '—'}
      </span>
    </div>
    <div class="actions">
      <button class="btn btn-ghost btn-icon btn-sm" title="Edit" onclick={() => edit(a)}><Icon name="edit" size={15} /></button>
      <button class="btn btn-ghost btn-icon btn-sm" title="Duplicate" onclick={() => duplicate(a)}><Icon name="copy" size={15} /></button>
      <button class="btn btn-ghost btn-icon btn-sm" title="Delete" onclick={() => remove(a)}><Icon name="trash" size={15} /></button>
    </div>
  </div>
{/snippet}

<style>
  .site-mark {
    width: 48px;
    height: 48px;
    border-radius: 14px;
    background: var(--site);
    color: #fff;
    display: grid;
    place-items: center;
    font-weight: 800;
    font-size: 1.35rem;
    flex: none;
  }
  .cooldown {
    margin-bottom: 1.25rem;
    align-items: center;
  }
  .header-actions {
    gap: 1.1rem;
    flex-wrap: wrap;
  }
  .site-switch {
    cursor: pointer;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
    margin-bottom: 1rem;
  }
  .filter {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0 0.7rem;
    height: 36px;
    width: 260px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-3);
  }
  .filter input {
    border: none;
    outline: none;
    background: transparent;
    font: inherit;
    color: var(--text);
    width: 100%;
  }
  .group {
    margin-bottom: 1rem;
    overflow: hidden;
  }
  .group header {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.8rem 1rem;
    background: var(--surface-2);
    border-bottom: 1px solid var(--border);
    color: var(--text-2);
  }
  .group header strong {
    color: var(--text);
  }
  .search-meta {
    gap: 0.9rem;
    margin-top: 0.15rem;
  }
  .sub {
    margin: 1.5rem 0 0.6rem;
    color: var(--text-2);
  }
  .alert-row {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
  }
  .alert-row:last-child {
    border-bottom: none;
  }
  .alert-row:hover {
    background: color-mix(in srgb, var(--surface-2) 50%, transparent);
  }
  .alert-row.disabled .alert-main {
    opacity: 0.6;
  }
  .alert-main {
    flex: 1;
    min-width: 0;
    text-align: left;
    border: none;
    background: none;
    padding: 0;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }
  .name {
    font-weight: 650;
  }
  .rules {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.3rem;
    margin-top: 0.3rem;
  }
  .side {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 0.15rem;
    white-space: nowrap;
  }
  .actions {
    display: flex;
    gap: 0.1rem;
  }
  .err {
    color: var(--danger);
  }
  .new {
    color: var(--success);
    font-weight: 600;
  }
  .nowrap {
    white-space: nowrap;
  }
  .table-wrap {
    overflow-x: auto;
  }
  .table td strong {
    max-width: 260px;
    display: inline-block;
  }
  .matches {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
    gap: 0.9rem;
  }
  @media (max-width: 760px) {
    .side {
      display: none;
    }
    .filter {
      width: 100%;
    }
  }
</style>
