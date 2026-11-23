<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '../lib/api';
  import { app, bootstrap, refreshStatus, setSiteEnabled, toast, toastError } from '../lib/state.svelte';
  import type { ChatInfo, Destination, NetworkSettings, TelegramSettings } from '../lib/types';
  import Icon from '../components/Icon.svelte';
  import HealthBadge from '../components/HealthBadge.svelte';
  import Toggle from '../components/Toggle.svelte';
  import { confirmDialog } from '../components/Confirm.svelte';

  let telegram = $state<TelegramSettings | null>(null);
  let destinations = $state<Destination[]>([]);
  let network = $state<Record<string, NetworkSettings>>({});
  let proxiesText = $state<Record<string, string>>({});

  // Telegram token
  let token = $state('');
  let savingToken = $state(false);

  // Destination form
  let destForm = $state<{ id: number; name: string; chatId: string } | null>(null);
  let chats = $state<ChatInfo[] | null>(null);
  let discovering = $state(false);
  let testing = $state<string | null>(null);

  // Security
  let currentPw = $state('');
  let newPw = $state('');

  async function load() {
    try {
      const [t, d, n] = await Promise.all([api.telegram(), api.destinations(), api.network()]);
      telegram = t;
      destinations = d;
      network = n;
      proxiesText = Object.fromEntries(Object.entries(n).map(([k, v]) => [k, v.proxies.join('\n')]));
    } catch (e) {
      toastError(e);
    }
  }
  onMount(load);

  async function saveToken() {
    savingToken = true;
    try {
      telegram = await api.updateTelegram({ token: token.trim() });
      token = '';
      toast(telegram.configured ? `Connected to @${telegram.botUsername}` : 'Telegram disconnected');
      refreshStatus();
    } catch (e) {
      toastError(e);
    } finally {
      savingToken = false;
    }
  }

  async function disconnect() {
    if (!(await confirmDialog({ title: 'Disconnect the bot?', message: 'No more Telegram messages will be sent until a token is set again.', confirmLabel: 'Disconnect', danger: true }))) return;
    try {
      telegram = await api.updateTelegram({ token: '' });
      refreshStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function discover() {
    discovering = true;
    try {
      chats = await api.discoverChats();
      if (chats.length === 0) toast('No chat found yet. Send /start to your bot (or post in your channel) and try again.', 'info');
    } catch (e) {
      toastError(e);
    } finally {
      discovering = false;
    }
  }

  function pickChat(c: ChatInfo) {
    if (!destForm) destForm = { id: 0, name: '', chatId: '' };
    destForm.chatId = c.id;
    if (!destForm.name) destForm.name = c.title;
  }

  async function saveDestination() {
    if (!destForm) return;
    try {
      if (destForm.id) await api.updateDestination({ id: destForm.id, name: destForm.name, chatId: destForm.chatId });
      else await api.createDestination({ name: destForm.name, chatId: destForm.chatId });
      toast(`Chat “${destForm.name}” saved`);
      destForm = null;
      chats = null;
      destinations = await api.destinations();
      refreshStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function removeDestination(d: Destination) {
    if (!(await confirmDialog({ title: 'Remove this chat?', message: `Alerts will stop sending messages to “${d.name}”.`, confirmLabel: 'Remove', danger: true }))) return;
    try {
      await api.deleteDestination(d.id);
      destinations = await api.destinations();
      telegram = await api.telegram();
      refreshStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function sendTest(chatId: string) {
    testing = chatId;
    try {
      await api.testTelegram(chatId);
      toast('Test message sent. Check Telegram!');
    } catch (e) {
      toastError(e);
    } finally {
      testing = null;
    }
  }

  async function setSystemDestination(id: number) {
    try {
      telegram = await api.updateTelegram({ systemDestinationId: id });
      toast(id ? 'System notices enabled' : 'System notices disabled', 'info');
    } catch (e) {
      toastError(e);
    }
  }

  async function saveNetwork(providerId: string) {
    const s = network[providerId];
    try {
      network[providerId] = await api.updateNetwork(providerId, {
        ...s,
        proxies: (proxiesText[providerId] ?? '').split('\n').map((p) => p.trim()).filter(Boolean),
      });
      toast('Network settings saved');
    } catch (e) {
      toastError(e);
    }
  }

  async function changePassword(remove = false) {
    try {
      const res = await api.changePassword(currentPw, remove ? '' : newPw);
      toast(res.required ? 'Password saved' : 'Password removed');
      currentPw = newPw = '';
      app.auth = await api.authStatus();
    } catch (e) {
      toastError(e);
    }
  }

  async function logout() {
    await api.logout();
    await bootstrap();
  }

  const isGroupChat = (id: string) => id.startsWith('-') || id.startsWith('@');
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1>Settings</h1>
      <p>Telegram delivery, network behaviour and access to this interface.</p>
    </div>
  </div>

  <nav class="jump-links">
    <a href="#/settings" onclick={(e) => { e.preventDefault(); document.getElementById('telegram')?.scrollIntoView({ behavior: 'smooth' }); }}>Telegram</a>
    <a href="#/settings" onclick={(e) => { e.preventDefault(); document.getElementById('sites')?.scrollIntoView({ behavior: 'smooth' }); }}>Sites</a>
    <a href="#/settings" onclick={(e) => { e.preventDefault(); document.getElementById('network')?.scrollIntoView({ behavior: 'smooth' }); }}>Network</a>
    <a href="#/settings" onclick={(e) => { e.preventDefault(); document.getElementById('security')?.scrollIntoView({ behavior: 'smooth' }); }}>Security</a>
  </nav>

  <!-- Telegram bot -->
  <section class="card card-pad block" id="telegram">
    <div class="block-head">
      <span class="block-icon"><Icon name="send" /></span>
      <div class="grow">
        <h2>Telegram bot</h2>
        <p class="text-2 small">ZVintedWatch sends matches through your own Telegram bot.</p>
      </div>
      {#if telegram?.configured}<span class="badge badge-success"><Icon name="check" size={12} stroke={3} /> @{telegram.botUsername}</span>{/if}
    </div>

    {#if !telegram?.configured}
      <ol class="guide">
        <li>Open Telegram and start a chat with <a href="https://t.me/BotFather" target="_blank" rel="noopener noreferrer">@BotFather</a>.</li>
        <li>Send <code>/newbot</code>, choose a name and a username ending in “bot”.</li>
        <li>BotFather replies with a token like <code>123456789:AAH…</code>. Paste it below.</li>
      </ol>
    {/if}

    <div class="row wrap">
      <input class="input mono grow" type="password" bind:value={token} autocomplete="off"
        placeholder={telegram?.configured ? `Current token: ${telegram.tokenHint} — paste a new one to replace it` : 'Bot token'} />
      <button class="btn btn-primary" onclick={saveToken} disabled={!token.trim() || savingToken}>
        {#if savingToken}<span class="spinner"></span>{/if} {telegram?.configured ? 'Replace token' : 'Connect bot'}
      </button>
      {#if telegram?.configured}<button class="btn btn-ghost btn-danger" onclick={disconnect}>Disconnect</button>{/if}
    </div>
    <p class="hint">The token is stored only in your local database, inside the data folder.</p>
  </section>

  <!-- Destinations -->
  <section class="card card-pad block">
    <div class="block-head">
      <span class="block-icon"><Icon name="users" /></span>
      <div class="grow">
        <h2>Chats receiving alerts</h2>
        <p class="text-2 small">A private chat with you, or a group/channel to share alerts with several people.</p>
      </div>
      <button class="btn" onclick={() => (destForm = { id: 0, name: '', chatId: '' })} disabled={!telegram?.configured}>
        <Icon name="plus" size={15} /> Add chat
      </button>
    </div>

    {#if destinations.length}
      <div class="dest-list">
        {#each destinations as d (d.id)}
          <div class="dest">
            <Icon name={isGroupChat(d.chatId) ? 'users' : 'send'} size={16} />
            <div class="grow">
              <div class="dest-name">{d.name}</div>
              <div class="muted small mono">{d.chatId} · {isGroupChat(d.chatId) ? 'group / channel' : 'private chat'}</div>
            </div>
            <button class="btn btn-sm" onclick={() => sendTest(d.chatId)} disabled={testing === d.chatId}>
              {#if testing === d.chatId}<span class="spinner"></span>{:else}<Icon name="send" size={13} />{/if} Test
            </button>
            <button class="btn btn-sm btn-ghost btn-icon" title="Edit" onclick={() => (destForm = { id: d.id, name: d.name, chatId: d.chatId })}><Icon name="edit" size={15} /></button>
            <button class="btn btn-sm btn-ghost btn-icon" title="Remove" onclick={() => removeDestination(d)}><Icon name="trash" size={15} /></button>
          </div>
        {/each}
      </div>
    {:else if telegram?.configured && !destForm}
      <p class="muted">No chat yet. Add one to receive alerts.</p>
    {/if}

    {#if destForm}
      <div class="dest-form">
        <div class="callout">
          <Icon name="info" size={16} />
          <div>
            <strong>Private chat:</strong> open your bot in Telegram (<a href="https://t.me/{telegram?.botUsername}" target="_blank" rel="noopener noreferrer">@{telegram?.botUsername}</a>) and press <em>Start</em>.<br />
            <strong>Channel or group:</strong> add the bot as an administrator allowed to post, then post any message.<br />
            Then click <em>Find chats</em> and pick it — or type the id yourself.
          </div>
        </div>
        <div class="row wrap">
          <button class="btn" onclick={discover} disabled={discovering}>
            {#if discovering}<span class="spinner"></span>{:else}<Icon name="search" size={15} />{/if} Find chats
          </button>
          {#each chats ?? [] as c (c.id)}
            <button class="chip pick" class:active={destForm.chatId === c.id} onclick={() => pickChat(c)}>
              <Icon name={c.type === 'private' ? 'send' : 'users'} size={12} /> {c.title} <span class="muted">({c.type})</span>
            </button>
          {/each}
        </div>
        <div class="form-grid">
          <div class="field">
            <label for="dest-name">Name</label>
            <input id="dest-name" class="input" bind:value={destForm.name} placeholder="e.g. Me, Family channel" />
          </div>
          <div class="field">
            <label for="dest-chat">Chat id</label>
            <input id="dest-chat" class="input mono" bind:value={destForm.chatId} placeholder="123456789, -100… or @channel" />
          </div>
        </div>
        <div class="row">
          <button class="btn btn-primary" onclick={saveDestination} disabled={!destForm.name.trim() || !destForm.chatId.trim()}>Save chat</button>
          <button class="btn" onclick={() => sendTest(destForm!.chatId)} disabled={!destForm.chatId.trim()}>Send test message</button>
          <button class="btn btn-ghost" onclick={() => { destForm = null; chats = null; }}>Cancel</button>
        </div>
      </div>
    {/if}

    {#if destinations.length}
      <div class="field system">
        <label for="system-dest">System notices</label>
        <select id="system-dest" class="input" value={telegram?.systemDestinationId ?? 0}
          onchange={(e) => setSystemDestination(Number(e.currentTarget.value))}>
          <option value={0}>Don't send system notices</option>
          {#each destinations as d}<option value={d.id}>{d.name}</option>{/each}
        </select>
        <span class="hint">Get a message when a site starts refusing requests and ZVintedWatch pauses.</span>
      </div>
    {/if}
  </section>

  <!-- Sites -->
  <section class="card card-pad block" id="sites">
    <div class="block-head">
      <span class="block-icon"><Icon name="layers" /></span>
      <div class="grow">
        <h2>Sites</h2>
        <p class="text-2 small">
          Each site is an independent module. Pausing a site stops all its requests; its alerts are kept and the
          other sites are not affected.
        </p>
      </div>
    </div>
    <div class="dest-list">
      {#each app.meta?.providers ?? [] as p (p.id)}
        {@const st = app.status?.providers.find((x) => x.id === p.id)}
        <div class="dest">
          <span class="dot" style="background: {p.color}"></span>
          <div class="grow">
            <div class="dest-name">{p.name}</div>
            <div class="muted small">
              {st?.enabledAlerts ?? 0}/{st?.alerts ?? 0} alerts enabled ·
              <a href={p.website} target="_blank" rel="noopener noreferrer">{p.website.replace(/^https?:\/\//, '')}</a>
            </div>
          </div>
          <span class="small text-2">{st?.enabled === false ? 'Paused' : 'Active'}</span>
          <Toggle checked={st?.enabled ?? true} label={st?.enabled === false ? `Resume ${p.name}` : `Pause ${p.name}`}
            onchange={(v) => setSiteEnabled(p.id, v)} />
        </div>
      {/each}
    </div>
  </section>

  <!-- Network -->
  <section class="card card-pad block" id="network">
    <div class="block-head">
      <span class="block-icon"><Icon name="globe" /></span>
      <div class="grow">
        <h2>Network</h2>
        <p class="text-2 small">How politely each site is queried. Defaults are safe for most uses.</p>
      </div>
    </div>
    {#each app.meta?.providers ?? [] as p (p.id)}
      {@const s = network[p.id]}
      {@const st = app.status?.providers.find((x) => x.id === p.id)}
      {#if s}
        <div class="net">
          <div class="row">
            <span class="dot" style="background: {p.color}"></span>
            <h3 class="grow">{p.name}</h3>
            <HealthBadge health={st?.health} />
          </div>
          <div class="form-grid three">
            <div class="field">
              <label for="gap-{p.id}">Minimum gap between requests</label>
              <div class="unit"><input id="gap-{p.id}" class="input" type="number" min="1000" step="500" bind:value={s.minGapMs} /><span>ms</span></div>
            </div>
            <div class="field">
              <label for="jitter-{p.id}">Random extra delay</label>
              <div class="unit"><input id="jitter-{p.id}" class="input" type="number" min="0" step="500" bind:value={s.jitterMs} /><span>ms</span></div>
            </div>
            <div class="field">
              <label for="timeout-{p.id}">Request timeout</label>
              <div class="unit"><input id="timeout-{p.id}" class="input" type="number" min="5" bind:value={s.timeoutSec} /><span>s</span></div>
            </div>
          </div>
          <div class="field">
            <label for="proxies-{p.id}">Proxies <span class="muted small">(optional, one per line)</span></label>
            <textarea id="proxies-{p.id}" class="input mono" rows="3" bind:value={proxiesText[p.id]}
              placeholder={'socks5://127.0.0.1:1080\nhttp://user:password@proxy.example.com:8080'}></textarea>
            <span class="hint">
              Requests are spread over the proxies and each one is paced separately. Tip: to make requests leave from another
              machine (e.g. a seedbox), open an SSH tunnel with <code>ssh -D 1080 user@host</code> and use
              <code>socks5://127.0.0.1:1080</code>. Datacenter IPs are more likely to be blocked than home connections.
            </span>
          </div>
          <div class="row">
            <button class="btn btn-primary" onclick={() => saveNetwork(p.id)}>Save {p.name} settings</button>
          </div>
        </div>
      {/if}
    {/each}
  </section>

  <!-- Security -->
  <section class="card card-pad block" id="security">
    <div class="block-head">
      <span class="block-icon"><Icon name="lock" /></span>
      <div class="grow">
        <h2>Security</h2>
        <p class="text-2 small">
          {#if app.auth?.required}
            This interface is protected by a password.
          {:else}
            No password: anyone who can open this address can use the interface.
            {#if app.meta?.loopback}That is fine while it only listens on this computer (127.0.0.1).{/if}
          {/if}
        </p>
      </div>
      {#if app.auth?.required}<button class="btn btn-ghost" onclick={logout}><Icon name="logout" size={15} /> Log out</button>{/if}
    </div>
    <div class="form-grid">
      {#if app.auth?.required}
        <div class="field">
          <label for="pw-current">Current password</label>
          <input id="pw-current" class="input" type="password" bind:value={currentPw} autocomplete="current-password" />
        </div>
      {/if}
      <div class="field">
        <label for="pw-new">{app.auth?.required ? 'New password' : 'Set a password'}</label>
        <input id="pw-new" class="input" type="password" bind:value={newPw} autocomplete="new-password" placeholder="At least 8 characters" />
      </div>
    </div>
    <div class="row">
      <button class="btn btn-primary" onclick={() => changePassword()} disabled={newPw.length < 8}>Save password</button>
      {#if app.auth?.required && app.meta?.loopback}
        <button class="btn btn-ghost btn-danger" onclick={() => changePassword(true)} disabled={!currentPw}>Remove password</button>
      {/if}
    </div>
  </section>

  <p class="about muted small">
    ZVintedWatch {app.meta?.version} · open source ·
    <a href="https://github.com/Nikola-Putnik/ZVintedWatch" target="_blank" rel="noopener noreferrer">documentation & source code</a>
  </p>
</div>

<style>
  .jump-links {
    display: flex;
    gap: 1rem;
    margin-bottom: 1rem;
    font-weight: 600;
    font-size: 0.92em;
  }
  .block {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    margin-bottom: 1.25rem;
    scroll-margin-top: 1rem;
  }
  .block-head {
    display: flex;
    align-items: center;
    gap: 0.85rem;
  }
  .block-icon {
    width: 40px;
    height: 40px;
    border-radius: 12px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
    flex: none;
  }
  .guide {
    margin: 0;
    padding: 0.8rem 1rem 0.8rem 2.2rem;
    border-radius: var(--radius);
    background: var(--surface-2);
    display: grid;
    gap: 0.35rem;
    color: var(--text-2);
  }
  .dest-list {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
  }
  .dest {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.7rem 0.9rem;
    border-bottom: 1px solid var(--border);
    color: var(--text-2);
  }
  .dest:last-child {
    border-bottom: none;
  }
  .dest-name {
    font-weight: 600;
    color: var(--text);
  }
  .dest-form {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    padding: 1rem;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius);
  }
  .pick {
    cursor: pointer;
    font: inherit;
    font-size: 0.85rem;
  }
  .pick.active {
    background: var(--accent-soft);
    color: var(--accent-text);
    border-color: transparent;
  }
  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.85rem;
  }
  .form-grid.three {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .system {
    max-width: 420px;
  }
  .net {
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    padding: 1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .unit {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--text-3);
  }
  .about {
    text-align: center;
    margin-top: 1.5rem;
  }
  @media (max-width: 760px) {
    .form-grid,
    .form-grid.three {
      grid-template-columns: 1fr;
    }
  }
</style>
