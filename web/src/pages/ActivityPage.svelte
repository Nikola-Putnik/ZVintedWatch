<script lang="ts">
  import { tick } from 'svelte';
  import { activity, clearActivity } from '../lib/activity.svelte';
  import { app } from '../lib/state.svelte';
  import type { ActivityEvent, EventKind } from '../lib/types';
  import { clockTime, formatBytes, shortUrl } from '../lib/format';
  import Icon from '../components/Icon.svelte';

  const kinds: { id: EventKind; label: string }[] = [
    { id: 'request', label: 'Requests' },
    { id: 'poll', label: 'Refreshes' },
    { id: 'match', label: 'Matches' },
    { id: 'notify', label: 'Telegram' },
    { id: 'system', label: 'System' },
  ];

  let enabled = $state<Record<string, boolean>>({ request: true, poll: true, match: true, notify: true, system: true });
  let site = $state('');
  let query = $state('');
  let errorsOnly = $state(false);
  let paused = $state(false);
  let follow = $state(true);
  let frozen = $state<ActivityEvent[] | null>(null);
  let expanded = $state<Record<number, boolean>>({});
  let consoleEl: HTMLDivElement | undefined = $state();

  const source = $derived(paused && frozen ? frozen : activity.events);
  const lines = $derived(
    source.filter((e) => {
      const kind = e.kind === 'health' ? 'system' : e.kind;
      if (!enabled[kind]) return false;
      if (site && e.provider && e.provider !== site) return false;
      if (errorsOnly && e.level !== 'warn' && e.level !== 'error') return false;
      if (query) {
        const q = query.toLowerCase();
        const hay = `${e.message} ${e.search ?? ''} ${e.http?.url ?? ''}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    }),
  );

  function togglePause() {
    paused = !paused;
    frozen = paused ? [...activity.events] : null;
  }

  // Keep the newest line in view while following.
  $effect(() => {
    lines.length;
    if (follow && consoleEl) {
      tick().then(() => consoleEl && (consoleEl.scrollTop = consoleEl.scrollHeight));
    }
  });

  function onscroll() {
    if (!consoleEl) return;
    const atBottom = consoleEl.scrollHeight - consoleEl.scrollTop - consoleEl.clientHeight < 30;
    if (follow !== atBottom) follow = atBottom;
  }

  function tagOf(e: ActivityEvent): string {
    switch (e.kind) {
      case 'request':
        return e.http ? e.http.method : 'HTTP';
      case 'poll':
        return 'POLL';
      case 'match':
        return 'MATCH';
      case 'notify':
        return 'TELEGRAM';
      case 'health':
        return 'HEALTH';
      default:
        return 'SYSTEM';
    }
  }

  function statusClass(code: number): string {
    if (!code) return 'st-err';
    if (code < 300) return 'st-ok';
    if (code < 400) return 'st-redirect';
    return 'st-err';
  }

  const providerName = (id?: string) => (id ? (app.meta?.providers.find((p) => p.id === id)?.name ?? id) : '');
</script>

<div class="page activity-page">
  <div class="page-header">
    <div>
      <h1>Live activity</h1>
      <p>Every request sent to the marketplaces and what came back, in real time.</p>
    </div>
    <div class="row">
      <span class="conn" class:on={activity.connected}>
        <span class="dot"></span>{activity.connected ? 'Live' : 'Reconnecting…'}
      </span>
    </div>
  </div>

  <div class="toolbar card">
    <div class="kinds">
      {#each kinds as k}
        <button class="kind" class:on={enabled[k.id]} onclick={() => (enabled[k.id] = !enabled[k.id])}>{k.label}</button>
      {/each}
    </div>
    <select class="input site" bind:value={site} aria-label="Site">
      <option value="">All sites</option>
      {#each app.meta?.providers ?? [] as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <label class="row small errors"><input type="checkbox" bind:checked={errorsOnly} /> Warnings &amp; errors only</label>
    <div class="search">
      <Icon name="filter" size={14} />
      <input bind:value={query} placeholder="Filter text" aria-label="Filter text" />
    </div>
    <div class="row grow end">
      <button class="btn btn-sm" onclick={togglePause} title={paused ? 'Resume' : 'Freeze the display'}>
        <Icon name={paused ? 'play' : 'pause'} size={14} /> {paused ? 'Resume' : 'Pause'}
      </button>
      <button class="btn btn-sm btn-ghost" onclick={() => { clearActivity(); frozen = paused ? [] : null; }} title="Clear the console">
        <Icon name="trash" size={14} /> Clear
      </button>
    </div>
  </div>

  <div class="console" bind:this={consoleEl} {onscroll}>
    {#each lines as e (e.id)}
      <div class="line {e.level} k-{e.kind}" class:open={expanded[e.id]}>
        <button class="summary" onclick={() => (expanded[e.id] = !expanded[e.id])}>
          <span class="t">{clockTime(e.time)}</span>
          <span class="tag">{tagOf(e)}</span>
          {#if e.provider}<span class="prov">{providerName(e.provider)}</span>{/if}
          {#if e.kind === 'request' && e.http}
            <span class="code {statusClass(e.http.status)}">{e.http.status || 'ERR'}</span>
            <span class="url">{shortUrl(e.http.url)}</span>
            <span class="meta">{e.http.durationMs} ms{#if e.http.bytes} · {formatBytes(e.http.bytes)}{/if}</span>
          {:else}
            <span class="msg">{e.message}</span>
          {/if}
          {#if e.search}<span class="search-label">{e.search}</span>{/if}
        </button>
        {#if expanded[e.id]}
          <div class="details">
            {#if e.http}
              <div><span class="k">url</span> {e.http.url}</div>
              <div><span class="k">via</span> {e.http.lane ?? 'direct'}</div>
              {#if e.http.error}<div class="error-text"><span class="k">error</span> {e.http.error}</div>{/if}
              <div><span class="k">note</span> {e.message}</div>
            {/if}
            {#each e.details ?? [] as d}<div>{d}</div>{/each}
            {#if !e.http && !(e.details ?? []).length}<div class="dim">No details.</div>{/if}
          </div>
        {/if}
      </div>
    {:else}
      <div class="placeholder">Waiting for activity… Requests appear here as soon as an alert refreshes.</div>
    {/each}
  </div>

  {#if !follow}
    <button class="btn btn-primary jump" onclick={() => { follow = true; consoleEl && (consoleEl.scrollTop = consoleEl.scrollHeight); }}>
      <Icon name="arrowDown" size={15} /> Jump to latest
    </button>
  {/if}
</div>

<style>
  .activity-page {
    max-width: none;
    height: 100vh;
    display: flex;
    flex-direction: column;
    padding-bottom: 1.5rem;
    position: relative;
  }
  .page-header {
    margin-bottom: 1rem;
  }
  .conn {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-3);
  }
  .conn .dot {
    background: var(--text-3);
  }
  .conn.on {
    color: var(--success);
  }
  .conn.on .dot {
    background: var(--success);
    animation: pulse 1.6s infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.75rem;
    padding: 0.6rem 0.75rem;
    margin-bottom: 0.75rem;
  }
  .kinds {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .kind {
    border: 1px solid var(--border-strong);
    background: var(--surface);
    color: var(--text-3);
    border-radius: 999px;
    padding: 0.2rem 0.7rem;
    font: inherit;
    font-size: 0.83rem;
    font-weight: 600;
    cursor: pointer;
  }
  .kind.on {
    background: var(--accent-soft);
    color: var(--accent-text);
    border-color: transparent;
  }
  .site {
    width: 140px;
    min-height: 32px;
    padding: 0.25rem 0.5rem;
  }
  .errors {
    gap: 0.35rem;
    color: var(--text-2);
    cursor: pointer;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    padding: 0 0.55rem;
    height: 32px;
    color: var(--text-3);
    background: var(--surface);
  }
  .search input {
    border: none;
    outline: none;
    background: transparent;
    font: inherit;
    color: var(--text);
    width: 150px;
  }
  .end {
    justify-content: flex-end;
  }

  .console {
    flex: 1;
    min-height: 300px;
    overflow-y: auto;
    background: var(--console-bg);
    color: var(--console-text);
    border-radius: var(--radius-lg);
    padding: 0.6rem 0;
    font-family: var(--mono);
    font-size: 12.5px;
    line-height: 1.55;
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.04);
  }
  .line .summary {
    display: flex;
    align-items: baseline;
    gap: 0.75rem;
    width: 100%;
    padding: 0.08rem 1rem;
    border: none;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    white-space: nowrap;
    overflow: hidden;
  }
  .line .summary:hover {
    background: rgba(255, 255, 255, 0.045);
  }
  .t {
    color: var(--console-dim);
    flex: none;
  }
  .tag {
    flex: none;
    width: 68px;
    font-weight: 700;
    color: #8fa3ff;
  }
  .k-poll .tag {
    color: #c39bff;
  }
  .k-match .tag,
  .k-match .msg {
    color: #4ee39a;
  }
  .k-notify .tag {
    color: #4cc3ff;
  }
  .k-system .tag,
  .k-health .tag {
    color: #c9cdd8;
  }
  .prov {
    flex: none;
    color: #7ad3d8;
  }
  .code {
    flex: none;
    font-weight: 700;
  }
  .st-ok {
    color: #4ee39a;
  }
  .st-redirect {
    color: #ffd166;
  }
  .st-err {
    color: #ff6b6b;
  }
  .url {
    color: #e7e9f0;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .meta {
    flex: none;
    color: var(--console-dim);
  }
  .msg {
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .search-label {
    margin-left: auto;
    flex: none;
    max-width: 30%;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--console-dim);
  }
  .warn .msg,
  .warn .tag {
    color: #ffd166;
  }
  .error .msg,
  .error .tag {
    color: #ff6b6b;
  }
  .details {
    margin: 0.15rem 1rem 0.45rem 6.4rem;
    padding: 0.45rem 0.7rem;
    border-left: 2px solid #3a3f55;
    color: #b9bed0;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .details .k {
    display: inline-block;
    width: 3.2rem;
    color: var(--console-dim);
  }
  .error-text {
    color: #ff6b6b;
  }
  .dim {
    color: var(--console-dim);
  }
  .placeholder {
    padding: 1rem;
    color: var(--console-dim);
  }
  .jump {
    position: absolute;
    right: 3rem;
    bottom: 3rem;
    box-shadow: var(--shadow-lg);
  }
  @media (max-width: 760px) {
    .activity-page {
      height: calc(100vh - 52px);
    }
    .search-label,
    .meta {
      display: none;
    }
    .details {
      margin-left: 1rem;
    }
  }
</style>
