<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    content,
    side = 'top',
    class: className,
    children,
  }: {
    content: Snippet;
    side?: 'top' | 'right' | 'bottom' | 'left';
    class?: string;
    children: Snippet;
  } = $props();

  let visible = $state(false);
  let triggerEl: HTMLElement | undefined = $state();

  const uid = $props.id();
  const tooltipId = `${uid}-tooltip`;

  const FOCUSABLE = 'a[href],button,input,select,textarea,[tabindex]:not([tabindex="-1"])';

  // role="tooltip" associates nothing on its own — the link is an
  // aria-describedby on the trigger, and the trigger is the consumer's snippet,
  // so it can only be wired imperatively. Only while the tooltip is mounted: an
  // aria-describedby pointing at a missing id reads as no description at all.
  $effect(() => {
    if (!visible || !triggerEl) return;
    const target = triggerEl.querySelector<HTMLElement>(FOCUSABLE) ?? triggerEl;
    target.setAttribute('aria-describedby', tooltipId);
    return () => target.removeAttribute('aria-describedby');
  });

  // WCAG 2.1 SC 1.4.13: content shown on hover must be dismissible without
  // moving the pointer. Hovering keeps it dismissed until the pointer leaves
  // and comes back, which is the intent.
  function dismiss(e: KeyboardEvent) {
    if (e.key === 'Escape') visible = false;
  }

  const positions = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
    right: 'left-full top-1/2 -translate-y-1/2 ml-2',
    bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
    left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  };
</script>

<svelte:window onkeydown={visible ? dismiss : undefined} />

<!-- The pointer handlers sit on the wrapper, not the trigger: the trigger is the
     consumer's snippet, and the wrapper has to enclose the tooltip so moving the
     pointer onto it keeps it open (WCAG 2.1 SC 1.4.13, hoverable). focusin and
     focusout give keyboard users the same reveal, so nothing is gated on hover. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<span
  class="relative inline-flex"
  bind:this={triggerEl}
  onmouseenter={() => (visible = true)}
  onmouseleave={() => (visible = false)}
  onfocusin={() => (visible = true)}
  onfocusout={() => (visible = false)}
>
  {@render children()}
  {#if visible}
    <!-- A <span>, not a <div>: the wrapper is a <span>, which only accepts
         phrasing content. Absolute positioning blockifies it either way. -->
    <span
      id={tooltipId}
      role="tooltip"
      class={cn(
        'absolute z-popover max-w-xs rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md',
        positions[side],
        className,
      )}
    >
      {@render content()}
    </span>
  {/if}
</span>
