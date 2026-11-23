<script lang="ts">
  import { api } from '../lib/api';
  import { toast } from '../lib/state.svelte';
  import type { Alert, Destination, PreviewResult, ProviderInfo, SearchInfo } from '../lib/types';
  import { formatPrice, formatSeconds, relativeTime } from '../lib/format';
  import Drawer from './Drawer.svelte';
  import ChipInput from './ChipInput.svelte';
  import Toggle from './Toggle.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    provider: ProviderInfo;
    alert: Alert;
    destinations: Destination[];
    onclose: () => void;
    onsaved: () => void;
  }
  let { provider, alert, destinations, onclose, onsaved }: Props = $props();

  // Work on a deep copy so cancelling leaves the original untouched. The editor
  // is re-created for every alert, so capturing the initial value is intended.
  // svelte-ignore state_referenced_locally
  let draft = $state<Alert>(structuredClone($state.snapshot(alert)) as Alert);
  // svelte-ignore state_referenced_locally
  const isNew = !alert.id;

  let saving = $state(false);
  let error = $state('');

  // --- Search ------------------------------------------------------------
  const isURL = $derived(/^(https?:\/\/|www\.|[a-z0-9-]+\.[a-z.]+\/)/i.test(draft.searchInput.trim()));
  let searchInfo = $state<SearchInfo | null>(null);
  let searchError = $state('');

  $effect(() => {
    const input = draft.searchInput;
    const domain = draft.domain;
    if (!input.trim()) {
      searchInfo = null;
      searchError = '';
      return;
    }
    const timer = setTimeout(async () => {
      try {
        searchInfo = await api.describeSearch(provider.id, input, domain, draft.id);
        searchError = '';
      } catch (e) {
        searchInfo = null;
        searchError = e instanceof Error ? e.message : String(e);
      }
    }, 350);
    return () => clearTimeout(timer);
  });

  // --- Interval ----------------------------------------------------------
  let randomInterval = $state(draft.intervalMinSec !== draft.intervalMaxSec);

  function setFixed(sec: number) {
    randomInterval = false;
    draft.intervalMinSec = draft.intervalMaxSec = sec;
  }
  function setRandom(min: number, max: number) {
    randomInterval = true;
    draft.intervalMinSec = min;
    draft.intervalMaxSec = max;
  }
  function switchMode(random: boolean) {
    if (random === randomInterval) return;
    if (random) setRandom(draft.intervalMinSec, Math.max(draft.intervalMinSec * 3, draft.intervalMinSec + 30));
    else setFixed(draft.intervalMinSec);
  }
  $effect(() => {
    if (!randomInterval) draft.intervalMaxSec = draft.intervalMinSec;
  });

  const presets = [
    { label: '30 s', min: 30, max: 30 },
    { label: '1 min', min: 60, max: 60 },
    { label: '2 min', min: 120, max: 120 },
    { label: '5 min', min: 300, max: 300 },
    { label: '30 s – 3 min', min: 30, max: 180 },
    { label: '1 – 5 min', min: 60, max: 300 },
  ];

  const avgInterval = $derived((draft.intervalMinSec + draft.intervalMaxSec) / 2 || 1);
  const tooFast = $derived(draft.intervalMinSec < provider.recommendedIntervalSec);
  const belowFloor = $derived(draft.intervalMinSec < provider.minIntervalSec);

  // --- Price ---------------------------------------------------------------
  function priceValue(v: string): number | null {
    const n = parseFloat(v.replace(',', '.'));
    return Number.isFinite(n) && n >= 0 ? n : null;
  }

  // --- Live keyword test --------------------------------------------------
  let sampleTitle = $state('');
  let sampleResult = $state<boolean | null>(null);
  $effect(() => {
    const text = sampleTitle;
    const filter = $state.snapshot(draft.title);
    if (!text.trim()) {
      sampleResult = null;
      return;
    }
    const timer = setTimeout(async () => {
      try {
        sampleResult = (await api.testMatcher(filter, text)).match;
      } catch {
        sampleResult = null;
      }
    }, 250);
    return () => clearTimeout(timer);
  });

  // --- Preview -----------------------------------------------------------
  let preview = $state<PreviewResult | null>(null);
  let previewing = $state(false);
  let showAll = $state(false);

  async function runPreview() {
    previewing = true;
    error = '';
    try {
      preview = await api.previewAlert($state.snapshot(draft) as Alert);
      showAll = false;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      previewing = false;
    }
  }
  const previewItems = $derived(
    preview ? (showAll ? preview.items : preview.items.filter((i) => i.match)) : [],
  );

  // --- Save --------------------------------------------------------------
  function toggleDestination(id: number) {
    draft.destinationIds = draft.destinationIds.includes(id)
      ? draft.destinationIds.filter((d) => d !== id)
      : [...draft.destinationIds, id];
  }

  async function save() {
    saving = true;
    error = '';
    const payload = $state.snapshot(draft) as Alert;
    if (!payload.name.trim()) payload.name = searchInfo?.label.replace(/[“”]/g, '').split(' · ')[0] || 'New alert';
    try {
      if (isNew) await api.createAlert(payload);
      else await api.updateAlert(payload);
      toast(isNew ? `Alert “${payload.name}” created` : `Alert “${payload.name}” saved`);
      onsaved();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }
</script>

<Drawer title={isNew ? 'New alert' : 'Edit alert'} subtitle={provider.name} {onclose}>
  <form class="editor" onsubmit={(e) => { e.preventDefault(); save(); }}>
    <!-- Name -->
    <section>
      <div class="field">
        <label for="alert-name">Alert name</label>
        <input id="alert-name" class="input" bind:value={draft.name} placeholder="e.g. God of War PS5 under 30 €" maxlength="120" />
        <span class="hint">Shown in Telegram messages. Leave empty to use the search keywords.</span>
      </div>
    </section>

    <!-- Search -->
    <section>
      <div class="section-head">
        <span class="step">1</span>
        <div>
          <h3>What to search</h3>
          <p class="hint">{provider.searchHelp}</p>
        </div>
      </div>
      <div class="search-row">
        <input
          class="input grow"
          class:invalid={!!searchError}
          bind:value={draft.searchInput}
          placeholder={provider.searchPlaceholder}
          spellcheck="false"
          aria-label="Search URL or keywords"
        />
        {#if !isURL && provider.domains.length > 1}
          <select class="input domain" bind:value={draft.domain} aria-label="Country">
            {#each provider.domains as d}<option value={d}>{d}</option>{/each}
          </select>
        {/if}
      </div>
      {#if searchError}
        <p class="field-error">{searchError}</p>
      {:else if searchInfo}
        <div class="search-info">
          <Icon name="search" size={15} />
          <span class="grow">
            Watching <strong>{searchInfo.label}</strong>
            <a href={searchInfo.webUrl} target="_blank" rel="noopener noreferrer" title="Open this search on {provider.name}"><Icon name="external" size={13} /></a>
          </span>
          {#if searchInfo.sharedWith.length}
            <span class="badge badge-success" title={searchInfo.sharedWith.map((a) => a.name).join(', ')}>
              <Icon name="layers" size={12} /> Shared with {searchInfo.sharedWith.length} alert{searchInfo.sharedWith.length > 1 ? 's' : ''} — no extra request
            </span>
          {:else}
            <span class="badge" title="Estimated requests added by this alert">+~{Math.round(3600 / avgInterval)} requests/hour</span>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Title rules -->
    <section>
      <div class="section-head">
        <span class="step">2</span>
        <div>
          <h3>Title keywords</h3>
          <p class="hint">
            Case and accents are ignored. Use <code>a|b</code> for alternatives, <code>word*</code> as a wildcard,
            <code>/regex/</code> for advanced patterns. Press Enter to add a term.
          </p>
        </div>
      </div>
      <div class="field">
        <div class="row">
          <span class="label grow">Must contain</span>
          <div class="segmented" role="group" aria-label="Include mode">
            <button type="button" class:active={draft.title.includeMode !== 'any'} onclick={() => (draft.title.includeMode = 'all')}>All terms</button>
            <button type="button" class:active={draft.title.includeMode === 'any'} onclick={() => (draft.title.includeMode = 'any')}>Any term</button>
          </div>
        </div>
        <ChipInput bind:values={draft.title.include} tone="include" placeholder="e.g. god of war|gow, ragnarok" />
      </div>
      <div class="field">
        <span class="label">Must not contain</span>
        <ChipInput bind:values={draft.title.exclude} tone="exclude" placeholder="e.g. empty box, ps4" />
      </div>
      <div class="tester">
        <Icon name="eye" size={15} />
        <input class="grow" bind:value={sampleTitle} placeholder="Try a listing title to test these rules…" />
        {#if sampleResult === true}<span class="badge badge-success"><Icon name="check" size={12} stroke={3} /> Matches</span>{/if}
        {#if sampleResult === false}<span class="badge badge-danger"><Icon name="x" size={12} stroke={3} /> No match</span>{/if}
      </div>
    </section>

    <!-- Description rules -->
    <section>
      <div class="section-head">
        <span class="step">3</span>
        <div>
          <h3>Description keywords <span class="muted small">(optional)</span></h3>
          <p class="hint">
            {#if provider.descriptionInSearch}
              Descriptions come with the search results, at no extra cost.
            {:else}
              To save requests, the description is only read when a new listing already passes the title and price rules (one extra request per candidate).
            {/if}
          </p>
        </div>
      </div>
      <div class="field">
        <div class="row">
          <span class="label grow">Must contain</span>
          <div class="segmented" role="group" aria-label="Description include mode">
            <button type="button" class:active={draft.description.includeMode !== 'any'} onclick={() => (draft.description.includeMode = 'all')}>All terms</button>
            <button type="button" class:active={draft.description.includeMode === 'any'} onclick={() => (draft.description.includeMode = 'any')}>Any term</button>
          </div>
        </div>
        <ChipInput bind:values={draft.description.include} tone="include" placeholder="e.g. sealed|neuf" />
      </div>
      <div class="field">
        <span class="label">Must not contain</span>
        <ChipInput bind:values={draft.description.exclude} tone="exclude" placeholder="e.g. broken, for parts" />
      </div>
    </section>

    <!-- Price -->
    <section>
      <div class="section-head">
        <span class="step">4</span>
        <div>
          <h3>Price range <span class="muted small">(optional)</span></h3>
          <p class="hint">Checked locally, so alerts with different budgets can share the same search.</p>
        </div>
      </div>
      <div class="row price">
        <input class="input" type="text" inputmode="decimal" placeholder="Min"
          value={draft.priceMin ?? ''} oninput={(e) => (draft.priceMin = priceValue(e.currentTarget.value))} aria-label="Minimum price" />
        <span class="muted">to</span>
        <input class="input" type="text" inputmode="decimal" placeholder="Max"
          value={draft.priceMax ?? ''} oninput={(e) => (draft.priceMax = priceValue(e.currentTarget.value))} aria-label="Maximum price" />
      </div>
    </section>

    <!-- Interval -->
    <section>
      <div class="section-head">
        <span class="step">5</span>
        <div>
          <h3>Refresh interval</h3>
          <p class="hint">A random interval looks less robotic. Alerts sharing a search are refreshed at the pace of the most frequent one.</p>
        </div>
      </div>
      <div class="row wrap">
        <div class="segmented" role="group" aria-label="Interval mode">
          <button type="button" class:active={!randomInterval} onclick={() => switchMode(false)}>Fixed</button>
          <button type="button" class:active={randomInterval} onclick={() => switchMode(true)}>Random range</button>
        </div>
        <div class="row interval">
          {#if randomInterval}
            <span class="muted small">between</span>
          {:else}
            <span class="muted small">every</span>
          {/if}
          <input class="input" type="number" min={provider.minIntervalSec} bind:value={draft.intervalMinSec} aria-label="Minimum seconds" />
          {#if randomInterval}
            <span class="muted small">and</span>
            <input class="input" type="number" min={draft.intervalMinSec} bind:value={draft.intervalMaxSec} aria-label="Maximum seconds" />
          {/if}
          <span class="muted small">seconds</span>
        </div>
      </div>
      <div class="presets">
        {#each presets as p}
          <button type="button" class="chip preset"
            class:active={draft.intervalMinSec === p.min && draft.intervalMaxSec === p.max}
            onclick={() => (p.min === p.max ? setFixed(p.min) : setRandom(p.min, p.max))}>{p.label}</button>
        {/each}
      </div>
      <p class="hint">
        {#if randomInterval}
          Refreshes every {formatSeconds(draft.intervalMinSec || 0)} to {formatSeconds(draft.intervalMaxSec || 0)}.
        {:else}
          Refreshes every {formatSeconds(draft.intervalMinSec || 0)}.
        {/if}
      </p>
      {#if belowFloor}
        <div class="callout danger"><Icon name="alert" size={16} /> The minimum is {provider.minIntervalSec} seconds.</div>
      {:else if tooFast}
        <div class="callout warning">
          <Icon name="alert" size={16} />
          <span>Refreshing faster than every {formatSeconds(provider.recommendedIntervalSec)} increases the risk of being temporarily blocked by {provider.name}, especially with many searches.</span>
        </div>
      {/if}
    </section>

    <!-- Destinations -->
    <section>
      <div class="section-head">
        <span class="step">6</span>
        <div>
          <h3>Send matches to</h3>
          <p class="hint">A listing matching several alerts is sent only once per chat.</p>
        </div>
      </div>
      {#if destinations.length === 0}
        <div class="callout warning">
          <Icon name="send" size={16} />
          <span>No Telegram chat configured yet. Matches will still be listed here. <a href="#/settings">Set up Telegram</a></span>
        </div>
      {:else}
        <div class="destinations">
          {#each destinations as d (d.id)}
            <label class="dest" class:checked={draft.destinationIds.includes(d.id)}>
              <input type="checkbox" checked={draft.destinationIds.includes(d.id)} onchange={() => toggleDestination(d.id)} />
              <Icon name={d.chatId.startsWith('-') || d.chatId.startsWith('@') ? 'users' : 'send'} size={15} />
              <span class="grow ellipsis">{d.name}</span>
            </label>
          {/each}
        </div>
      {/if}
    </section>

    <!-- Preview -->
    <section class="preview-section">
      <div class="row wrap">
        <div class="grow">
          <h3>Preview</h3>
          <p class="hint">Check which of the current listings these rules would keep.</p>
        </div>
        <button type="button" class="btn" onclick={runPreview} disabled={previewing || !draft.searchInput.trim()}>
          {#if previewing}<span class="spinner"></span>{:else}<Icon name="play" size={14} />{/if}
          Preview current listings
        </button>
      </div>
      {#if preview}
        <div class="preview">
          <div class="preview-summary">
            <strong>{preview.matches}</strong> of {preview.items.length} current listings pass the title and price rules.
            {#if preview.descriptionRules}<span class="muted">Description rules are checked for new listings only.</span>{/if}
            <span class="muted small">
              {preview.cached ? 'Reused results from ' + relativeTime(preview.fetchedAt) + ' (no request sent).' : 'Fetched just now.'}
            </span>
          </div>
          {#each previewItems as it (it.listing.id)}
            <a class="preview-item" class:no={!it.match} href={it.listing.url} target="_blank" rel="noopener noreferrer">
              {#if it.listing.photoUrl}<img src={it.listing.photoUrl} alt="" loading="lazy" referrerpolicy="no-referrer" />{:else}<span class="ph"></span>{/if}
              <span class="grow ellipsis" title={it.listing.title}>{it.listing.title}</span>
              <span class="price">{formatPrice(it.listing.price, it.listing.currency)}</span>
              {#if it.match}
                <Icon name="check" size={16} stroke={3} class="ok" />
              {:else}
                <span class="why">{!it.titleOk ? 'title' : 'price'}</span>
              {/if}
            </a>
          {:else}
            <p class="muted small">No current listing matches. That is fine: alerts fire on new listings.</p>
          {/each}
          {#if preview.items.length > preview.matches}
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => (showAll = !showAll)}>
              {showAll ? 'Hide non-matching listings' : `Show the ${preview.items.length - preview.matches} other listings`}
            </button>
          {/if}
        </div>
      {/if}
    </section>
  </form>

  {#snippet footer()}
    <label class="row enabled">
      <Toggle bind:checked={draft.enabled} label="Enabled" />
      <span class="small text-2">{draft.enabled ? 'Enabled' : 'Paused'}</span>
    </label>
    {#if error}<span class="footer-error grow" title={error}><Icon name="alert" size={15} /> {error}</span>{:else}<span class="grow"></span>{/if}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={save} disabled={saving || belowFloor || !draft.searchInput.trim()}>
      {#if saving}<span class="spinner"></span>{/if}
      {isNew ? 'Create alert' : 'Save changes'}
    </button>
  {/snippet}
</Drawer>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 1.1rem;
  }
  section {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 1.1rem 1.2rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
  }
  .section-head {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
  }
  .section-head .hint {
    margin-top: 0.15rem;
  }
  .step {
    flex: none;
    width: 24px;
    height: 24px;
    border-radius: 7px;
    display: grid;
    place-items: center;
    font-size: 0.8rem;
    font-weight: 700;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .search-row {
    display: flex;
    gap: 0.5rem;
  }
  .domain {
    width: 150px;
    flex: none;
  }
  .search-info {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem;
    padding: 0.55rem 0.75rem;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    font-size: 0.9em;
    color: var(--text-2);
  }
  .field-error {
    color: var(--danger);
    font-size: 0.88em;
  }
  .tester {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.45rem 0.7rem;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-sm);
    color: var(--text-3);
  }
  .tester input {
    border: none;
    outline: none;
    background: transparent;
    font: inherit;
    color: var(--text);
  }
  .price .input {
    width: 120px;
  }
  .interval .input {
    width: 90px;
  }
  .presets {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .preset {
    cursor: pointer;
    font: inherit;
    font-size: 0.83rem;
  }
  .preset.active {
    background: var(--accent-soft);
    color: var(--accent-text);
    border-color: transparent;
  }
  .destinations {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 0.5rem;
  }
  .dest {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-weight: 550;
  }
  .dest.checked {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .preview {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }
  .preview-summary {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.6rem 0.75rem;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    font-size: 0.9em;
    margin-bottom: 0.35rem;
  }
  .preview-item {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    padding: 0.35rem 0.4rem;
    border-radius: var(--radius-sm);
    color: var(--text);
    text-decoration: none !important;
    font-size: 0.9em;
  }
  .preview-item:hover {
    background: var(--surface-2);
  }
  .preview-item.no {
    opacity: 0.55;
  }
  .preview-item img,
  .preview-item .ph {
    width: 36px;
    height: 36px;
    object-fit: cover;
    border-radius: 6px;
    background: var(--surface-3);
    flex: none;
  }
  .preview-item .price {
    font-weight: 650;
    white-space: nowrap;
  }
  .preview-item :global(.ok) {
    color: var(--success);
  }
  .why {
    font-size: 0.75rem;
    color: var(--danger);
    font-weight: 600;
    width: 34px;
    text-align: right;
  }
  .enabled {
    cursor: pointer;
  }
  .footer-error {
    color: var(--danger);
    font-size: 0.88em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  @media (max-width: 600px) {
    .search-row {
      flex-direction: column;
    }
    .domain {
      width: 100%;
    }
  }
</style>
