<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { Star, X } from '@lucide/svelte';
  import { githubStars, formatStars, GITHUB_URL } from '$lib/github.svelte';
  import { bannerVisible } from '$lib/consent.svelte';
  import { readDismissed, readPhBannerDismissed, shouldShow, writeDismissed } from '$lib/supportToast';

  // A floating ask for a GitHub star, self-gating so the layout mounts it unconditionally.
  //
  // It queues behind the Product Hunt strip rather than competing with it: `shouldShow`
  // holds it back until that strip has stopped asking. Unlike the strip, this surface is
  // `fixed` and so moves nothing — which is why it needs neither SSR nor the pre-paint
  // class in app.html, and why that file (and the CSP hash over its inline script) stays
  // untouched.
  //
  // It also yields the bottom-right corner to the consent banner, which wants the same
  // box: consent is an obligation, a promo is not. `bannerVisible` is rune-backed, so the
  // toast appears on its own the moment consent is settled — no timer, no subscription.

  // Starts false so nothing flashes before the gate has been read; storage is only
  // reachable after mount anyway.
  let allowed = $state(false);

  onMount(() => {
    allowed = shouldShow({
      now: Date.now(),
      phBannerDismissed: readPhBannerDismissed(),
      selfDismissed: readDismissed(),
    });
    if (allowed) void githubStars.load();
  });

  // /open is the open-source pitch in full; repeating it in a toast is noise.
  const visible = $derived(allowed && !bannerVisible() && page.url.pathname !== '/open');

  const count = $derived(githubStars.count);

  // Following the link answers the ask as surely as closing the toast does — someone who
  // went to star the repo must not be asked again.
  function dismiss() {
    allowed = false;
    writeDismissed();
  }
</script>

{#if visible}
  <div
    role="status"
    class="fixed inset-x-4 bottom-4 z-30 rounded-lg border border-border bg-background/95 px-4 py-3 shadow-lg backdrop-blur sm:inset-x-auto sm:right-4 sm:max-w-md"
  >
    <div class="flex items-start gap-3">
      <p class="min-w-0 flex-1 text-sm text-muted-foreground">
        <span class="font-medium text-foreground">freehire is open source.</span>
        Free to use, and it stays that way because people star it.
      </p>

      <button
        type="button"
        onclick={dismiss}
        aria-label="Dismiss"
        class="-mr-1 -mt-1 shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    </div>

    <!-- Block form, not disable-next-line: the tag spans several lines, so the href the
         rule fires on is not the line after the comment. -->
    <!-- eslint-disable svelte/no-navigation-without-resolve -- external GitHub URL, not an internal route -->
    <a
      href={GITHUB_URL}
      target="_blank"
      rel="noreferrer"
      onclick={dismiss}
      class="mt-3 inline-flex items-center gap-1.5 text-sm font-medium text-brand underline-offset-4 hover:underline"
    >
      <Star class="size-4" aria-hidden="true" />
      Star on GitHub
      {#if count != null}
        <span class="tabular-nums text-muted-foreground">{formatStars(count)}</span>
      {/if}
    </a>
    <!-- eslint-enable svelte/no-navigation-without-resolve -->
  </div>
{/if}
