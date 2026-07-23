<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';
  import { X } from '@lucide/svelte';

  let {
    open = $bindable(false),
    title,
    description,
    class: className,
    children,
  }: {
    open?: boolean;
    title?: string;
    description?: string;
    class?: string;
    children: Snippet;
  } = $props();

  function close() {
    open = false;
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && open) {
      e.preventDefault();
      close();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
  <div
    class="fixed inset-0 z-modal bg-black/50 backdrop-blur-sm"
    onclick={(e) => { if (e.currentTarget === e.target) close(); }}
    role="button"
    tabindex="-1"
    aria-label="Close dialog"
  ></div>
  <div
    role="dialog"
    aria-modal="true"
    aria-label={title}
    class={cn(
      'fixed left-1/2 top-1/2 z-popover w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-card p-6 shadow-lg',
      className,
    )}
  >
    {#if title}
      <h2 class="text-lg font-semibold">{title}</h2>
    {/if}
    {#if description}
      <p class="mt-1 text-sm text-muted-foreground">{description}</p>
    {/if}
    <div class="mt-4">
      {@render children()}
    </div>
    <button
      type="button"
      class="absolute right-4 top-4 rounded-sm opacity-70 transition-opacity hover:opacity-100"
      onclick={close}
    aria-label="Close"
  >
    <X class="size-4" />
  </button>
  </div>
{/if}
