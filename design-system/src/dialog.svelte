<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';
  import { X } from '@lucide/svelte';
  import { lockScroll, unlockScroll } from './scroll-lock.js';

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
    /** Override the card's maximum width; responsive classes size only wider screens. */
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
    if (open && !el.open) {
      el.showModal();
      // showModal() focuses the first focusable descendant (often a button below
      // the fold) and the platform scrolls it into view. Start at the title even when
      // the content needs scrolling, without touching the page scroll lockScroll owns.
      el.scrollTop = 0;
    }
    if (!open && el.open) el.close();
  });

  // The one thing showModal() does not do is stop the page behind from scrolling.
  $effect(() => {
    if (!open) return;
    lockScroll();
    return unlockScroll;
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

  // Preventing `cancel` is necessary and not sufficient. The close watcher
  // spends a user-activation budget on each preventDefault, and when it runs
  // out it fires `cancel` unprevented and closes regardless — in Chrome, two
  // Escapes are refused and the third gets through. That valve exists so a page
  // cannot trap someone, which is right in general and wrong for the seconds an
  // irreversible request is in flight, so a held dialog puts itself back.
  function onclose() {
    if (!dismissible) {
      el?.showModal();
      return;
    }
    open = false;
  }
</script>

<dialog
  bind:this={el}
  aria-labelledby={title ? titleId : description ? descriptionId : undefined}
  aria-describedby={title && description ? descriptionId : undefined}
  {onclose}
  {oncancel}
  {onclick}
  class={cn(
    // A content-sized card on every screen, with room around it and bounded scrolling.
    // fit-content avoids the native dialog's inset:0 stretching an auto-height box.
    'm-auto h-fit max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-lg overflow-y-auto overscroll-contain rounded-xl border border-border bg-card p-0 text-card-foreground shadow-lg outline-none backdrop:bg-black/50',
    'animate-modal-pop backdrop:animate-modal-fade',
    className,
  )}
>
  <div class="relative p-5 sm:p-6">
    {#if title}
      <h2 id={titleId} class="pr-8 text-base font-semibold sm:text-lg">{title}</h2>
    {/if}
    {#if description}
      <p id={descriptionId} class={cn('mt-2 text-sm leading-relaxed text-muted-foreground', !title && dismissible && 'pr-8')}>{description}</p>
    {/if}
    <div class="mt-4">
      {@render children()}
    </div>
    {#if dismissible}
      <button
        type="button"
        class="absolute right-3 top-3 flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onclick={() => (open = false)}
        aria-label="Close"
      >
        <X class="size-4" />
      </button>
    {/if}
  </div>
</dialog>
