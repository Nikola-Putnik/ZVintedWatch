<script lang="ts">
  import type { Match } from '../lib/types';
  import { formatPrice } from '../lib/format';
  import RelativeTime from './RelativeTime.svelte';
  import Icon from './Icon.svelte';

  let { match }: { match: Match } = $props();
  const l = $derived(match.listing);
  const facts = $derived([l.brand, l.size, l.condition, l.category].filter(Boolean).join(' · '));
  const delivery = $derived(
    {
      sent: { cls: 'badge-success', text: 'Sent' },
      pending: { cls: 'badge-accent', text: 'Sending' },
      failed: { cls: 'badge-danger', text: 'Not sent' },
      none: { cls: '', text: 'No chat' },
    }[match.delivery] ?? { cls: '', text: match.delivery },
  );
</script>

<a class="match card" href={l.url} target="_blank" rel="noopener noreferrer">
  <div class="photo">
    {#if l.photoUrl}
      <img src={l.photoUrl} alt="" loading="lazy" referrerpolicy="no-referrer" />
    {:else}
      <Icon name="tag" size={28} />
    {/if}
    <span class="price">{formatPrice(l.price, l.currency)}</span>
  </div>
  <div class="info">
    <div class="title" title={l.title}>{l.title}</div>
    {#if facts}<div class="facts ellipsis" title={facts}>{facts}</div>{/if}
    {#if l.location}<div class="facts ellipsis" title={l.location}>📍 {l.location}</div>{/if}
    <div class="meta">
      <span class="alert-name ellipsis" title={match.alertName}><Icon name="bell" size={12} /> {match.alertName}</span>
      <span class="badge {delivery.cls}">{delivery.text}</span>
    </div>
    <div class="time muted small"><RelativeTime time={match.matchedAt} /></div>
  </div>
</a>

<style>
  .match {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    color: inherit;
    text-decoration: none !important;
    transition: transform 0.12s, box-shadow 0.12s;
  }
  .match:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow);
  }
  .photo {
    position: relative;
    aspect-ratio: 4 / 4.4;
    background: var(--surface-2);
    display: grid;
    place-items: center;
    color: var(--text-3);
  }
  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .price {
    position: absolute;
    left: 0.5rem;
    bottom: 0.5rem;
    background: rgba(10, 12, 20, 0.78);
    color: #fff;
    font-weight: 700;
    font-size: 0.88rem;
    padding: 0.15rem 0.5rem;
    border-radius: 6px;
  }
  .info {
    padding: 0.65rem 0.75rem 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .title {
    font-weight: 600;
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    min-height: 2.6em;
  }
  .facts {
    color: var(--text-2);
    font-size: 0.84rem;
  }
  .meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.4rem;
    margin-top: 0.2rem;
  }
  .alert-name {
    font-size: 0.82rem;
    color: var(--accent-text);
    font-weight: 600;
  }
</style>
