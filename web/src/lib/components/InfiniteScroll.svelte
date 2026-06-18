<script lang="ts">
  // Bottom-of-list sentinel that calls `onLoad` once it scrolls into view, so the
  // next page loads when the user reaches the end — no "Load more" click. It is only
  // a trigger: the caller owns the Paginator and renders an accessible fallback
  // control (the LoadMore button) for keyboard/screen-reader users and retries.
  //
  // `rootMargin: 0px` is deliberate — load when the sentinel actually enters the
  // viewport ("when you hit the bottom"), not pre-fetched a screen ahead. `enabled`
  // gates observation so we don't fire while a page is in flight or none remain.
  let { onLoad, enabled }: { onLoad: () => void; enabled: boolean } = $props();

  let sentinel: HTMLElement | undefined = $state();

  $effect(() => {
    if (!enabled || !sentinel) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) onLoad();
      },
      { rootMargin: '0px' },
    );
    io.observe(sentinel);
    return () => io.disconnect();
  });
</script>

<div bind:this={sentinel} aria-hidden="true" class="h-px w-full"></div>
