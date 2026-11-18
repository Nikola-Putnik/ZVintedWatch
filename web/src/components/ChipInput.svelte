<script lang="ts">
  // Editable list of keyword terms. Press Enter (or type a comma) to add the
  // current term; Backspace on an empty input removes the last one.
  import Icon from './Icon.svelte';

  interface Props {
    values: string[];
    placeholder?: string;
    tone?: 'include' | 'exclude' | 'neutral';
    id?: string;
  }
  let { values = $bindable(), placeholder = '', tone = 'neutral', id }: Props = $props();
  let draft = $state('');

  function commit() {
    const v = draft.trim().replace(/,$/, '').trim();
    if (v && !values.includes(v)) values = [...values, v];
    draft = '';
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commit();
    } else if (e.key === 'Backspace' && draft === '' && values.length) {
      values = values.slice(0, -1);
    }
  }

  function oninput() {
    // A comma separates terms, except inside a /regular expression/.
    if (draft.endsWith(',') && !draft.trimStart().startsWith('/')) commit();
  }

  function remove(i: number) {
    values = values.filter((_, j) => j !== i);
  }
</script>

<div class="chips {tone}">
  {#each values as v, i (v)}
    <span class="term">
      <span class="text" title={v}>{v}</span>
      <button type="button" aria-label="Remove {v}" onclick={() => remove(i)}><Icon name="x" size={12} stroke={2.5} /></button>
    </span>
  {/each}
  <input
    {id}
    bind:value={draft}
    {onkeydown}
    {oninput}
    onblur={commit}
    placeholder={values.length ? '' : placeholder}
    autocomplete="off"
    spellcheck="false"
  />
</div>

<style>
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    align-items: center;
    min-height: 40px;
    padding: 0.35rem 0.45rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    cursor: text;
  }
  .chips:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }
  input {
    flex: 1;
    min-width: 140px;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    padding: 0.2rem 0.25rem;
  }
  .term {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    max-width: 100%;
    padding: 0.15rem 0.3rem 0.15rem 0.55rem;
    border-radius: 6px;
    font-size: 0.88rem;
    font-weight: 550;
    background: var(--surface-2);
    color: var(--text);
  }
  .include .term {
    background: var(--success-soft);
    color: var(--success);
  }
  .exclude .term {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 22rem;
  }
  .term button {
    display: inline-flex;
    border: none;
    background: transparent;
    color: inherit;
    opacity: 0.7;
    cursor: pointer;
    padding: 2px;
    border-radius: 4px;
  }
  .term button:hover {
    opacity: 1;
    background: rgba(0, 0, 0, 0.08);
  }
</style>
