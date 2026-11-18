<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    icon?: string;
    title: string;
    text?: string;
    children?: Snippet;
  }
  let { icon = 'sparkle', title, text = '', children }: Props = $props();
</script>

<div class="empty card">
  <div class="icon-wrap"><Icon name={icon} size={24} /></div>
  <h3>{title}</h3>
  {#if text}<p class="text-2">{text}</p>{/if}
  {#if children}<div class="actions">{@render children()}</div>{/if}
</div>

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: 0.5rem;
    padding: 2.5rem 1.5rem;
    border-style: dashed;
    box-shadow: none;
  }
  .icon-wrap {
    width: 48px;
    height: 48px;
    border-radius: 14px;
    display: grid;
    place-items: center;
    background: var(--accent-soft);
    color: var(--accent-text);
    margin-bottom: 0.4rem;
  }
  p {
    max-width: 30rem;
  }
  .actions {
    margin-top: 0.7rem;
    display: flex;
    gap: 0.6rem;
  }
</style>
