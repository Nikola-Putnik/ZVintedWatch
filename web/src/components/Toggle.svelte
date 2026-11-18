<script lang="ts">
  interface Props {
    checked: boolean;
    label?: string;
    disabled?: boolean;
    onchange?: (checked: boolean) => void;
  }
  let { checked = $bindable(), label = '', disabled = false, onchange }: Props = $props();

  function toggle() {
    if (disabled) return;
    checked = !checked;
    onchange?.(checked);
  }
</script>

<button
  type="button"
  class="toggle"
  class:on={checked}
  role="switch"
  aria-checked={checked}
  aria-label={label || undefined}
  title={label || undefined}
  {disabled}
  onclick={toggle}
>
  <span class="knob"></span>
</button>

<style>
  .toggle {
    position: relative;
    width: 38px;
    height: 22px;
    flex: none;
    border-radius: 999px;
    border: none;
    background: var(--border-strong);
    cursor: pointer;
    transition: background 0.15s;
    padding: 0;
  }
  .toggle.on {
    background: var(--success);
  }
  .toggle:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .knob {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
    transition: transform 0.15s;
  }
  .on .knob {
    transform: translateX(16px);
  }
</style>
