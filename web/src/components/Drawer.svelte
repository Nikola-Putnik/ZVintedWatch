<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  interface Props {
    title: string;
    subtitle?: string;
    onclose: () => void;
    children: Snippet;
    footer?: Snippet;
    width?: string;
  }
  let { title, subtitle = '', onclose, children, footer, width = '720px' }: Props = $props();

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose();
  }
</script>

<svelte:window {onkeydown} />

<div class="overlay" onclick={onclose} role="presentation"></div>
<div class="drawer" style="--w: {width}" role="dialog" aria-modal="true" aria-label={title}>
  <header>
    <div class="grow">
      <h2>{title}</h2>
      {#if subtitle}<p class="muted small">{subtitle}</p>{/if}
    </div>
    <button class="btn btn-ghost btn-icon" onclick={onclose} aria-label="Close"><Icon name="x" /></button>
  </header>
  <div class="body">
    {@render children()}
  </div>
  {#if footer}
    <footer>{@render footer()}</footer>
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(10, 12, 25, 0.45);
    backdrop-filter: blur(2px);
    z-index: 40;
    animation: fade 0.15s ease-out;
  }
  .drawer {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(var(--w), 100vw);
    background: var(--bg);
    z-index: 41;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-lg);
    animation: slide 0.2s ease-out;
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: 1rem;
    padding: 1.1rem 1.4rem;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .body {
    flex: 1;
    overflow-y: auto;
    padding: 1.4rem;
  }
  footer {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.9rem 1.4rem;
    border-top: 1px solid var(--border);
    background: var(--surface);
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  @keyframes slide {
    from {
      transform: translateX(40px);
      opacity: 0;
    }
  }
</style>
