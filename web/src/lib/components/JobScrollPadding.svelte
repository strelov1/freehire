<script lang="ts">
  // Renders nothing: while JobView is the page it reserves the edges its sticky bars cover.
  // `closed` (a closed posting has no apply bar); `signInPrompt` and `feedbackEl` are the
  // phone's sign-in offer above that bar, whose height the bottom edge grows by.
  let {
    closed,
    signInPrompt,
    feedbackEl,
  }: { closed: boolean; signInPrompt: boolean; feedbackEl: HTMLDivElement | undefined } = $props();

  // Keyboard focus must not land under a sticky bar (WCAG 2.4.11). Three of them cover the
  // page's edges here: the site header (`h-14`, every width), the pinned header (lg, see
  // JobPinnedHeader) and the phone's sticky apply bar. `scroll-padding` on the root is
  // what the browser honours when it scrolls a newly focused element into view, so the
  // page reserves both edges while it is mounted — on <html>, which is the scroller, and
  // only for as long as this view is the page. The bottom edge is reserved only while the
  // apply bar exists, which a closed posting does not render. The sizes are in the
  // <style> block below, beside the media query that switches them.
  $effect(() => {
    const root = document.documentElement;
    root.classList.add('job-scroll-pad');
    root.classList.toggle('job-scroll-pad-bar', !closed);
    const feedback = feedbackEl;
    if (!closed && signInPrompt && feedback) {
      const updateFeedbackHeight = () => {
        const parent = feedback.parentElement;
        const gap = parent ? Number.parseFloat(getComputedStyle(parent).rowGap) || 0 : 0;
        root.style.setProperty(
          '--job-scroll-pad-feedback-height',
          `${feedback.getBoundingClientRect().height + gap}px`,
        );
      };
      updateFeedbackHeight();
      const observer = new ResizeObserver(updateFeedbackHeight);
      observer.observe(feedback);
      return () => {
        observer.disconnect();
        root.classList.remove('job-scroll-pad', 'job-scroll-pad-bar');
        root.style.removeProperty('--job-scroll-pad-feedback-height');
      };
    }
    root.style.removeProperty('--job-scroll-pad-feedback-height');
    return () => root.classList.remove('job-scroll-pad', 'job-scroll-pad-bar');
  });
</script>

<style>
  /* See the effect above. Below lg: the site header on top (h-14 plus air), and
     the apply bar at the bottom — its pt-3, an h-11 button, a padding of at least pb-3 that
     grows with the home indicator, and a rule. From lg: the site header plus the pinned
     header under it (py-2.5 around an h-9 button, and a rule); no bottom bar. */
  :global(html.job-scroll-pad) {
    scroll-padding-top: 4rem;
  }

  :global(html.job-scroll-pad.job-scroll-pad-bar) {
    scroll-padding-bottom: calc(4.75rem + env(safe-area-inset-bottom) + var(--job-scroll-pad-feedback-height, 0px));
  }

  @media (min-width: 64rem) {
    :global(html.job-scroll-pad) {
      scroll-padding-top: 7.75rem;
    }

    :global(html.job-scroll-pad.job-scroll-pad-bar) {
      scroll-padding-bottom: 0;
    }
  }
</style>
