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
  // The floating content, so a tap can be told apart from a tap on the trigger.
  let contentEl: HTMLElement | undefined = $state();

  // The floating content sits outside the wrapper's own layout box (it's
  // `position: absolute`, offset by the `positions` margin below), so the pointer
  // crosses a real gap — not a descendant of the wrapper — on its way from the
  // trigger to the content. A bare mouseleave fires the instant the pointer enters
  // that gap, closing the tooltip before it arrives. Delaying the hide, and
  // cancelling the delay if the pointer lands back inside the wrapper (trigger or
  // content — both are covered by its mouseenter) within the grace period, is what
  // actually satisfies the "hoverable" guarantee below.
  const HIDE_DELAY_MS = 150;
  let hideTimer: ReturnType<typeof setTimeout> | undefined;

  function show() {
    clearTimeout(hideTimer);
    visible = true;
  }

  function scheduleHide() {
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      visible = false;
    }, HIDE_DELAY_MS);
  }

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
    if (e.key === 'Escape') hide();
  }

  // A touch pointer never hovers and rarely focuses, so without this the tooltip
  // is unreachable on a phone — mouse and keyboard readers get the content and
  // touch readers get nothing at all. Tap toggles instead.
  //
  // Only touch. A mouse and a pen both hover, so hover has already opened the
  // tooltip and toggling here would shut it under the pointer aiming at it —
  // "not a mouse" would have caught the pen by accident.
  //
  // A tap inside the tooltip's OWN content is ignored for a sharper version of
  // the same reason. The content is a descendant of this wrapper, so it reaches
  // this handler — and closing there unmounts the link before the click that
  // follows the pointerdown can land on it, so the tap does nothing at all.
  function toggleOnTouch(e: PointerEvent) {
    if (e.pointerType !== 'touch') return;
    if (contentEl?.contains(e.target as Node)) return;
    if (visible) hide();
    else show();
  }

  // The touch half of "dismissible": there is no pointer to move away, so the
  // next tap elsewhere is what closes it.
  //
  // The `contains` guard carries two things. It stops the opening tap from also
  // being the closing one (Svelte attaches this listener after the current event
  // finishes, so that should not happen anyway — the guard costs a line and does
  // not depend on that ordering holding). And because the floating content is a
  // DESCENDANT of triggerEl, it is also what keeps a tap on a link inside the
  // tooltip from dismissing the tooltip out from under the click that follows.
  function dismissOutside(e: PointerEvent) {
    if (!triggerEl?.contains(e.target as Node)) hide();
  }

  // Tab out of the trigger lands on whatever the content holds — it is next in DOM
  // order and rendered while visible. A bare hide() ran first, removing that element
  // mid-focus, so focus fell to <body> and focusin never re-opened: a keyboard reader
  // could read the description and never reach the link inside it.
  function hideOnFocusOut(e: FocusEvent) {
    if (!triggerEl?.contains(e.relatedTarget as Node)) hide();
  }

  function hide() {
    clearTimeout(hideTimer);
    visible = false;
  }

  // A pending hide left running past unmount would set state on a dead component.
  $effect(() => () => clearTimeout(hideTimer));

  const positions = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
    right: 'left-full top-1/2 -translate-y-1/2 ml-2',
    bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
    left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  };
</script>

<svelte:window
  onkeydown={visible ? dismiss : undefined}
  onpointerdown={visible ? dismissOutside : undefined}
/>

<!-- The pointer handlers sit on the wrapper, not the trigger: the trigger is the
     consumer's snippet, and the wrapper has to enclose the tooltip so moving the
     pointer onto it keeps it open (WCAG 2.1 SC 1.4.13, hoverable). focusin and
     focusout give keyboard users the same reveal, so nothing is gated on hover. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<span
  class="relative inline-flex"
  bind:this={triggerEl}
  onmouseenter={show}
  onmouseleave={scheduleHide}
  onfocusin={show}
  onfocusout={hideOnFocusOut}
  onpointerdown={toggleOnTouch}
>
  {@render children()}
  {#if visible}
    <!-- A <span>, not a <div>: the wrapper is a <span>, which only accepts
         phrasing content. Absolute positioning blockifies it either way.

         `w-max` (not just `max-w-xs`) is load-bearing on a small trigger. Left
         at `width: auto`, a `left-1/2 -translate-x-1/2`-positioned box is sized
         by CSS shrink-to-fit against the *containing block's* available width —
         here, the trigger span itself. A tiny icon-button trigger makes that
         available width tiny too, so long content collapsed to one word per
         line no matter how generous max-w-xs was. `w-max` sizes to the
         content's own max-content width instead, still capped by max-w-xs. -->
    <span
      id={tooltipId}
      bind:this={contentEl}
      role="tooltip"
      class={cn(
        'absolute z-popover w-max max-w-xs rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md',
        positions[side],
        className,
      )}
    >
      {@render content()}
    </span>
  {/if}
</span>
