<script lang="ts" module>
  // Minimal icon set (24×24, stroked). Each entry is a list of SVG path data.
  const icons: Record<string, string[]> = {
    home: ['M3 10.5 12 3l9 7.5', 'M5 9v11h5v-6h4v6h5V9'],
    bell: ['M6 9a6 6 0 0 1 12 0c0 6 2.5 8 2.5 8h-17S6 15 6 9z', 'M10 20a2.2 2.2 0 0 0 4 0'],
    terminal: ['M4 5h16v14H4z', 'm8 10 2.5 2L8 14', 'M13 15h3'],
    settings: ['M4 7h9', 'M17 7h3', 'M4 17h3', 'M11 17h9', 'M15 4.5v5', 'M9 14.5v5'],
    plus: ['M12 5v14', 'M5 12h14'],
    edit: ['M4 20h4L19 9l-4-4L4 16z', 'm13.5 6.5 4 4'],
    trash: ['M4 7h16', 'M10 11v6', 'M14 11v6', 'M6 7l1 13h10l1-13', 'M9 7V4h6v3'],
    copy: ['M9 9h11v11H9z', 'M5 15H4V4h11v1'],
    refresh: ['M20 11a8 8 0 1 0-2.3 5.7', 'M20 4v7h-7'],
    external: ['M14 4h6v6', 'M20 4 10 14', 'M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5'],
    check: ['M5 12.5 9.5 17 19 7'],
    x: ['M6 6l12 12', 'M18 6 6 18'],
    search: ['M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14z', 'm20 20-4-4'],
    send: ['M21 3 3 10.5l7 2.5 2.5 7z', 'M21 3 10 13'],
    clock: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M12 7v5l3 2'],
    chevronRight: ['m9 6 6 6-6 6'],
    chevronDown: ['m6 9 6 6 6-6'],
    sun: ['M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z', 'M12 2v2', 'M12 20v2', 'M4.9 4.9l1.4 1.4', 'M17.7 17.7l1.4 1.4', 'M2 12h2', 'M20 12h2', 'M4.9 19.1l1.4-1.4', 'M17.7 6.3l1.4-1.4'],
    moon: ['M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z'],
    menu: ['M4 6h16', 'M4 12h16', 'M4 18h16'],
    pause: ['M8 5v14', 'M16 5v14'],
    play: ['M7 4.5v15l12.5-7.5z'],
    alert: ['M12 3.5 2.5 20h19z', 'M12 10v4.5', 'M12 17.3v.2'],
    logout: ['M15 4h4v16h-4', 'm10 8-4 4 4 4', 'M6 12h10'],
    eye: ['M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z', 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z'],
    filter: ['M3 5h18l-7 8v6l-4 2v-8z'],
    tag: ['M3 12V4h8l10 10-8 8z', 'M7.5 8.5v.1'],
    lock: ['M5 11h14v10H5z', 'M8 11V7a4 4 0 0 1 8 0v4'],
    globe: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M3 12h18', 'M12 3c2.5 2.6 3.7 5.6 3.7 9s-1.2 6.4-3.7 9c-2.5-2.6-3.7-5.6-3.7-9S9.5 5.6 12 3z'],
    arrowDown: ['M12 5v14', 'm6 13 6 6 6-6'],
    zap: ['M13 2 4 14h7l-1 8 9-12h-7z'],
    info: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M12 11v5', 'M12 7.8v.2'],
    layers: ['m12 3 9 5-9 5-9-5z', 'm3 13 9 5 9-5'],
    sparkle: ['M12 3v4', 'M12 17v4', 'M3 12h4', 'M17 12h4', 'm6 6 2 2', 'm16 16 2 2', 'm18 6-2 2', 'm8 16-2 2'],
    users: ['M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z', 'M2 21v-1a6 6 0 0 1 12 0v1', 'M16 3.5a4 4 0 0 1 0 7', 'M22 21v-1a6 6 0 0 0-4-5.6'],
  };
</script>

<script lang="ts">
  interface Props {
    name: string;
    size?: number;
    stroke?: number;
    class?: string;
  }
  let { name, size = 18, stroke = 2, class: cls = '' }: Props = $props();
  const paths = $derived(icons[name] ?? icons.info);
</script>

<svg
  class="icon {cls}"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={stroke}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#each paths as d}
    <path {d} />
  {/each}
</svg>

<style>
  .icon {
    flex: none;
    display: inline-block;
    vertical-align: middle;
  }
</style>
