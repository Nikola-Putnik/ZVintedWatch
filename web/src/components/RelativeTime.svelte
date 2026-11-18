<script lang="ts" module>
  // A single shared clock refreshes every relative time once per second.
  const clock = $state({ now: Date.now() });
  setInterval(() => (clock.now = Date.now()), 1000);
</script>

<script lang="ts">
  import { relativeTime, isZeroTime } from '../lib/format';
  let { time, fallback = 'never' }: { time: string | undefined; fallback?: string } = $props();
</script>

<time datetime={time} title={isZeroTime(time) ? '' : new Date(time!).toLocaleString()}>
  {isZeroTime(time) ? fallback : relativeTime(time, clock.now)}
</time>
