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

  const uid = $props.id();
  const titleId = `${uid}-title`;
  const descriptionId = `${uid}-description`;

  let el: HTMLDialogElement | undefined = $state();

  // showModal() puts the dialog in the top layer and hands us the focus trap,
  // the inert background, Escape-to-close and focus restore. Reimplementing
  // any of that by hand is strictly worse.
  $effect(() => {
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  });

  // The one thing showModal() does not do is stop the page behind from scrolling.
  $effect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  });

  // The backdrop is a pseudo-element of the dialog, so clicks on it surface as
  // clicks on <dialog> itself; everything inside lands on the padded wrapper.
  function onclick(e: MouseEvent) {
    if (e.target === el) open = false;
  }
</script>

<dialog
  bind:this={el}
  aria-labelledby={title ? titleId : undefined}
  aria-describedby={description ? descriptionId : undefined}
  onclose={() => (open = false)}
  {onclick}
  class={cn(
    'm-auto w-full max-w-lg rounded-lg border border-border bg-card p-0 text-card-foreground shadow-lg backdrop:bg-black/50 backdrop:backdrop-blur-sm',
    className,
  )}
>
  <div class="relative p-6">
    {#if title}
      <h2 id={titleId} class="text-lg font-semibold">{title}</h2>
    {/if}
    {#if description}
      <p id={descriptionId} class="mt-1 text-sm text-muted-foreground">{description}</p>
    {/if}
    <div class="mt-4">
      {@render children()}
    </div>
    <button
      type="button"
      class="absolute right-4 top-4 rounded-sm opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      onclick={() => (open = false)}
      aria-label="Close"
    >
      <X class="size-4" />
    </button>
  </div>
</dialog>
