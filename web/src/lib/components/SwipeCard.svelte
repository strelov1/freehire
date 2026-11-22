<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { SwipeGesture, SwipeKind } from './swipeGesture.svelte.js';

  // The swipe deck's card frame: the stage, the next-card ghost, the dragged frame and the
  // decision stamps around the card body. The parent owns the queue and the fly-off timing,
  // and wraps this in {#key slug} so each card is a fresh node that plays the grow-in.
  let {
    gesture,
    label,
    hasNext,
    onCommit,
    children,
  }: {
    gesture: SwipeGesture;
    label: string;
    hasNext: boolean;
    onCommit: (kind: SwipeKind) => void;
    children: Snippet;
  } = $props();
</script>

<!-- Card stage: the card fills the available height, Tinder-style. -->
<main class="relative min-h-0 flex-1 px-4 py-3">
  <div class="relative mx-auto h-full w-full max-w-md select-none md:max-w-2xl">
    {#if hasNext}
      <div
        class="absolute inset-0 scale-95 rounded-3xl border border-border bg-card opacity-60"
        aria-hidden="true"
      ></div>
    {/if}

    <div
      role="group"
      aria-label={label}
      class="swipe-card absolute inset-0 flex flex-col overflow-hidden rounded-3xl border border-border bg-card shadow-xl duration-300 ease-out"
      class:transition={gesture.animate}
      style={`transform: translate3d(${gesture.dragX}px, 0, 0) rotate(${gesture.rotation}deg); opacity: ${gesture.opacity};`}
      onpointerdown={gesture.down}
      onpointermove={gesture.move}
      onpointerup={() => gesture.up(onCommit)}
      onpointercancel={() => gesture.reset()}
    >
      <!-- Swipe-direction stamps, fading in with drag distance. They anchor to the
           non-scrolling frame so they stay put as the content below scrolls. -->
      {#if gesture.dragX > 20}
        <span
          class="pointer-events-none absolute left-5 top-5 z-10 -rotate-12 rounded-lg border-4 border-brand px-3 py-1 text-2xl font-extrabold uppercase tracking-wider text-brand"
          style={`opacity: ${gesture.stampOpacity}`}
        >
          Save
        </span>
      {:else if gesture.dragX < -20}
        <span
          class="pointer-events-none absolute right-5 top-5 z-10 rotate-12 rounded-lg border-4 border-destructive px-3 py-1 text-2xl font-extrabold uppercase tracking-wider text-destructive"
          style={`opacity: ${gesture.stampOpacity}`}
        >
          Skip
        </span>
      {/if}

      {@render children()}
    </div>
  </div>
</main>

<style>
  /* Grow-in intro for each incoming card. Uses the independent `scale` property
     (not transform: scale) so it composes with the drag/fly-off translate+rotate
     on `transform` instead of fighting it. Re-triggers per card via the parent's {#key}. */
  .swipe-card {
    animation: card-in 300ms ease-out;
    /* Force a dedicated GPU compositing layer so per-frame drag updates only
       recomposite (no main-thread repaint) - kills the mobile drag jitter. The
       tilt pivots around the card's bottom for a natural Tinder-like arc. */
    will-change: transform;
    transform-origin: center bottom;
    backface-visibility: hidden;
  }

  @keyframes card-in {
    from {
      scale: 0.95;
      opacity: 0.6;
    }
    to {
      scale: 1;
      opacity: 1;
    }
  }

  /* Reduced motion keeps the card change visible as a plain fade: no grow-in here, and
     fling() drops the sweep and the tilt, leaving only the opacity transition. */
  @keyframes card-in-fade {
    from {
      opacity: 0.6;
    }
    to {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .swipe-card {
      animation-name: card-in-fade;
    }
  }
</style>
