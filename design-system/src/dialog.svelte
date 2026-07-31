<script lang="ts" module>
  // Counted, not captured per dialog: with two open, the inner one to close
  // would otherwise restore the overflow it read *after* the outer one had
  // already set it to hidden, leaving the page locked for good.
  let locks = 0;
  let restore = '';

  function lockPageScroll() {
    if (locks === 0) {
      restore = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
    }
    locks++;
    return () => {
      locks--;
      if (locks === 0) document.body.style.overflow = restore;
    };
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';
  import { X } from '@lucide/svelte';

  let {
    open = $bindable(false),
    title,
    description,
    dismissible = true,
    class: className,
    children,
  }: {
    open?: boolean;
    title?: string;
    description?: string;
    /**
     * Whether the caller may close it. Set false while an irreversible request
     * is in flight: the dialog holds the outcome, and Escape would hide whether
     * the thing succeeded. Escape, the backdrop and the close button all go
     * away together — leaving one of the three is a dialog that claims to be
     * held and is not.
     */
    dismissible?: boolean;
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
    return lockPageScroll();
  });

  // The backdrop is a pseudo-element of the dialog, so clicks on it surface as
  // clicks on <dialog> itself; everything inside lands on the padded wrapper.
  function onclick(e: MouseEvent) {
    if (e.target === el && dismissible) open = false;
  }

  // `cancel` is what showModal() fires for Escape, and preventing it is the
  // platform's own supported way to refuse. Reimplementing the refusal with a
  // keydown listener would fight the top layer instead of using it.
  function oncancel(e: Event) {
    if (!dismissible) e.preventDefault();
  }
</script>

<dialog
  bind:this={el}
  aria-labelledby={title ? titleId : undefined}
  aria-describedby={description ? descriptionId : undefined}
  onclose={() => (open = false)}
  {oncancel}
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
    {#if dismissible}
      <button
        type="button"
        class="absolute right-4 top-4 rounded-sm opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onclick={() => (open = false)}
        aria-label="Close"
      >
        <X class="size-4" />
      </button>
    {/if}
  </div>
</dialog>
