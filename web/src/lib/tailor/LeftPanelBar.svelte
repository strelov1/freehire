<script lang="ts" generics="T extends string">
  import { PanelLeftClose } from '@lucide/svelte';
  import { tablist } from '#lib/actions/tablist.js';

  // The tailoring workspace's left panel header at lg: its tabs (`current`), the editor's
  // autosave status while an editing tab is shown, and the control that folds the panel.
  let {
    tabs,
    current = $bindable(),
    saveState,
    saveError,
    oncollapse,
  }: {
    tabs: readonly (readonly [T, string])[];
    current: T;
    saveState: 'idle' | 'saving' | 'saved' | 'error';
    saveError: string;
    oncollapse: () => void;
  } = $props();
</script>

<div class="hidden items-center justify-between gap-2 border-b border-border px-2 py-1.5 text-sm lg:flex">
  <div
    role="tablist"
    aria-label="Edit this CV"
    use:tablist={current}
    class="flex min-w-0 flex-nowrap items-center gap-1 overflow-x-auto"
  >
    {#each tabs as [id, label] (id)}
      <button
        type="button"
        role="tab"
        id="tailor-left-tab-{id}"
        aria-selected={current === id}
        aria-controls="tailor-left-panel"
        onclick={() => (current = id)}
        class={[
          'shrink-0 rounded px-2 py-1 transition-colors',
          current === id
            ? 'bg-brand-muted font-semibold text-brand-strong'
            : 'text-muted-foreground hover:text-foreground',
        ]}
      >
        {label}
      </button>
    {/each}
  </div>
  <div class="flex shrink-0 items-center gap-1">
    {#if current === 'editor' || current === 'design'}
      <span
        class={['text-xs', saveState === 'error' ? 'text-destructive' : 'text-muted-foreground']}
        aria-live="polite"
        title={saveState === 'error' ? saveError : undefined}
      >
        {#if saveState === 'saving'}Saving…{:else if saveState === 'saved'}Saved{:else if saveState === 'error'}Save
          failed{/if}
      </span>
    {/if}
    <button
      type="button"
      onclick={oncollapse}
      aria-label="Collapse the editor panel"
      class="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      <PanelLeftClose class="size-4" />
    </button>
  </div>
</div>
