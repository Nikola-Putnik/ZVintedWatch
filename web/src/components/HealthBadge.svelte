<script lang="ts">
  import type { Health } from '../lib/types';
  import { clockTime } from '../lib/format';

  let { health }: { health: Health | undefined } = $props();

  const view = $derived.by(() => {
    switch (health?.state) {
      case 'cooldown':
        return { cls: 'badge-danger', text: `Cooling down until ${clockTime(health.cooldownUntil)}` };
      case 'degraded':
        return { cls: 'badge-warning', text: 'Some proxies cooling down' };
      case 'error':
        return { cls: 'badge-danger', text: 'Error' };
      default:
        return { cls: 'badge-success', text: 'Healthy' };
    }
  });
</script>

<span class="badge {view.cls}" title={health?.reason || 'Requests are going through normally'}>
  <span class="dot" style="background: currentColor"></span>
  {view.text}
</span>
