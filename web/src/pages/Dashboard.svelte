<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { app } from '../lib/state.svelte';
  import { activity } from '../lib/activity.svelte';
  import type { Match } from '../lib/types';
  import { clockTime, plural } from '../lib/format';
  import Icon from '../components/Icon.svelte';
  import HealthBadge from '../components/HealthBadge.svelte';
  import MatchCard from '../components/MatchCard.svelte';
  import EmptyState from '../components/EmptyState.svelte';

  let matches = $state<Match[]>([]);
  let loaded = $state(false);

  async function loadMatches() {
    try {
      matches = await api.matches({ limit: 12 });
    } catch {
      /* shown as empty */
    } finally {
      loaded = true;
    }
  }

  onMount(() => {
    loadMatches();
    const t = setInterval(loadMatches, 15000);
    return () => clearInterval(t);
  });

  const providers = $derived(app.status?.providers ?? []);
  const telegram = $derived(app.status?.telegram);
  const totals = $derived({
    alerts: providers.reduce((n, p) => n + p.enabledAlerts, 0),
    allAlerts: providers.reduce((n, p) => n + p.alerts, 0),
    searches: providers.reduce((n, p) => n + p.searches, 0),
    perHour: providers.reduce((n, p) => n + p.requestsPerHour, 0),
    today: providers.reduce((n, p) => n + p.matchesToday, 0),
  });
  const firstSite = $derived(app.meta?.providers[0]?.id ?? 'vinted');

  const steps = $derived([
    { done: !!telegram?.configured, title: 'Connect your Telegram bot', text: 'Paste the token given by @BotFather.', href: '#/settings' },
    { done: (telegram?.destinations ?? 0) > 0, title: 'Choose where alerts go', text: 'Your private chat or a channel shared with friends.', href: '#/settings' },
    { done: totals.allAlerts > 0, title: 'Create your first alert', text: 'Paste a search link or type keywords.', href: `#/site/${firstSite}` },
  ]);
  const onboarding = $derived(steps.some((s) => !s.done));

  const recentEvents = $derived(activity.events.filter((e) => e.kind !== 'request').slice(-8).reverse());
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1>Overview</h1>
      <p>
        {#if totals.searches > 0}
          Watching {plural(totals.searches, 'search', 'searches')} for {plural(totals.alerts, 'active alert')}
          — about {Math.round(totals.perHour)} requests per hour.
        {:else}
          Nothing is being watched yet.
        {/if}
      </p>
    </div>
  </div>

  {#if onboarding}
    <section class="card card-pad onboarding">
      <div class="row">
        <Icon name="sparkle" />
        <h2>Get started in three steps</h2>
      </div>
      <ol>
        {#each steps as s, i}
          <li class:done={s.done}>
            <span class="num">{#if s.done}<Icon name="check" size={14} stroke={3} />{:else}{i + 1}{/if}</span>
            <div class="grow">
              <div class="step-title">{s.title}</div>
              <div class="muted small">{s.text}</div>
            </div>
            {#if !s.done}<a class="btn btn-sm" href={s.href}>Open <Icon name="chevronRight" size={14} /></a>{/if}
          </li>
        {/each}
      </ol>
    </section>
  {/if}

  <section class="kpis">
    <div class="kpi card"><span class="kpi-label">Active alerts</span><span class="kpi-value">{totals.alerts}</span></div>
    <div class="kpi card"><span class="kpi-label">Watched searches</span><span class="kpi-value">{totals.searches}</span></div>
    <div class="kpi card"><span class="kpi-label">Requests / hour</span><span class="kpi-value">~{Math.round(totals.perHour)}</span></div>
    <div class="kpi card"><span class="kpi-label">Matches today</span><span class="kpi-value accent">{totals.today}</span></div>
  </section>

  <div class="section-title"><h2>Sites</h2></div>
  <section class="sites">
    {#each providers as p (p.id)}
      <a class="site card" href="#/site/{p.id}" style="--site: {p.color}">
        <div class="row">
          <span class="site-mark">{p.name.slice(0, 1)}</span>
          <div class="grow">
            <h3>{p.name}</h3>
            <HealthBadge health={p.health} />
          </div>
          <Icon name="chevronRight" />
        </div>
        <dl>
          <div><dt>Alerts</dt><dd>{p.enabledAlerts}<span class="muted">/{p.alerts}</span></dd></div>
          <div><dt>Searches</dt><dd>{p.searches}</dd></div>
          <div><dt>Requests last hour</dt><dd>{p.health.requestsLastHour}</dd></div>
          <div><dt>Matches today</dt><dd>{p.matchesToday}</dd></div>
        </dl>
      </a>
    {/each}
  </section>

  <div class="columns">
    <section>
      <div class="section-title"><h2>Latest matches</h2></div>
      {#if loaded && matches.length === 0}
        <EmptyState icon="bell" title="No match yet" text="Matching listings will appear here and in your Telegram chat as soon as they are posted." />
      {:else}
        <div class="matches">
          {#each matches as m (m.id)}
            <MatchCard match={m} />
          {/each}
        </div>
      {/if}
    </section>

    <section>
      <div class="section-title">
        <h2>Recent activity</h2>
        <a class="btn btn-sm btn-ghost" href="#/activity">Open console <Icon name="chevronRight" size={14} /></a>
      </div>
      <div class="card feed">
        {#each recentEvents as e (e.id)}
          <div class="event {e.level} {e.kind}">
            <span class="time mono">{clockTime(e.time)}</span>
            <div class="grow">
              {#if e.search}<div class="search ellipsis">{e.search}</div>{/if}
              <div class="msg">{e.message}</div>
            </div>
          </div>
        {:else}
          <p class="muted small empty-feed">No activity yet.</p>
        {/each}
      </div>
    </section>
  </div>
</div>

<style>
  .onboarding {
    margin-bottom: 1.5rem;
    background: linear-gradient(135deg, var(--accent-soft), var(--surface) 70%);
  }
  .onboarding .row {
    color: var(--accent-text);
  }
  .onboarding h2 {
    color: var(--text);
  }
  ol {
    list-style: none;
    margin: 1rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.6rem;
  }
  li {
    display: flex;
    align-items: center;
    gap: 0.85rem;
    padding: 0.7rem 0.85rem;
    border-radius: var(--radius);
    background: var(--surface);
    border: 1px solid var(--border);
  }
  li.done {
    opacity: 0.65;
  }
  li.done .step-title {
    text-decoration: line-through;
  }
  .num {
    width: 26px;
    height: 26px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-weight: 700;
    font-size: 0.85rem;
    background: var(--accent);
    color: #fff;
    flex: none;
  }
  li.done .num {
    background: var(--success);
  }
  .step-title {
    font-weight: 600;
  }

  .kpis {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 1rem;
  }
  .kpi {
    padding: 1rem 1.15rem;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }
  .kpi-label {
    color: var(--text-3);
    font-size: 0.82rem;
    font-weight: 600;
  }
  .kpi-value {
    font-size: 1.65rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .kpi-value.accent {
    color: var(--accent-text);
  }

  .sites {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 1rem;
  }
  .site {
    padding: 1.1rem 1.2rem;
    color: inherit;
    text-decoration: none !important;
    border-top: 3px solid var(--site);
    transition: box-shadow 0.12s, transform 0.12s;
  }
  .site:hover {
    box-shadow: var(--shadow);
    transform: translateY(-1px);
  }
  .site h3 {
    margin-bottom: 0.2rem;
  }
  .site-mark {
    width: 42px;
    height: 42px;
    border-radius: 12px;
    background: var(--site);
    color: #fff;
    display: grid;
    place-items: center;
    font-weight: 800;
    font-size: 1.2rem;
  }
  dl {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 0.5rem;
    margin: 1rem 0 0;
  }
  dt {
    font-size: 0.74rem;
    color: var(--text-3);
    font-weight: 600;
  }
  dd {
    margin: 0;
    font-weight: 700;
    font-size: 1.1rem;
  }

  .columns {
    display: grid;
    grid-template-columns: minmax(0, 1.7fr) minmax(0, 1fr);
    gap: 1.5rem;
  }
  .matches {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 0.85rem;
  }
  .feed {
    padding: 0.4rem 0;
  }
  .event {
    display: flex;
    gap: 0.75rem;
    padding: 0.55rem 1rem;
    border-bottom: 1px solid var(--border);
    font-size: 0.88em;
  }
  .event:last-child {
    border-bottom: none;
  }
  .time {
    color: var(--text-3);
    font-size: 0.85em;
    padding-top: 0.1em;
  }
  .search {
    font-size: 0.8em;
    color: var(--text-3);
    font-weight: 600;
  }
  .event.match .msg {
    color: var(--success);
    font-weight: 600;
  }
  .event.warn .msg {
    color: var(--warning);
  }
  .event.error .msg {
    color: var(--danger);
  }
  .empty-feed {
    padding: 1rem;
  }
  @media (max-width: 1100px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
  @media (max-width: 760px) {
    .kpis {
      grid-template-columns: repeat(2, 1fr);
    }
    dl {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
