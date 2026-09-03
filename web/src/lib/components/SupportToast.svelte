<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { Star, X } from '@lucide/svelte';
  import { githubStars, formatStars, GITHUB_URL } from '$lib/github.svelte';
  import { bannerVisible } from '$lib/consent.svelte';
  import { cn } from '$lib/ui';
  import {
    ownsMobileStickyCta,
    readDismissed,
    suppressesToast,
    writeDismissed,
  } from '$lib/supportToast';

  // A floating ask for a GitHub star, self-gating so the layout mounts it unconditionally.
  //
  // Unlike the CLI strip under the header, this surface is `fixed` and so moves nothing —
  // which is why it needs neither SSR nor the pre-paint class in app.html, and why that
  // file (and the CSP hash over its inline script) stays untouched.
  //
  // What it defers to is reactive: the consent banner, which owns the same corner, and
  // the route. Only the visitor's own answer is read once, on mount, because nothing else
  // in the session can change it.

  // Starts true so the gate is decided by mount-time storage rather than flashing first;
  // `mounted` keeps the toast off the server-rendered pass.
  let mounted = $state(false);
  let answered = $state(true);

  onMount(() => {
    answered = readDismissed();
    mounted = true;
    if (!answered) void githubStars.load();
  });

  const visible = $derived(
    mounted && !answered && !bannerVisible() && !suppressesToast(page.url.pathname),
  );

  const count = $derived(githubStars.count);

  // Following the link answers the ask as surely as closing the toast does — someone who
  // went to star the repo must not be asked again.
  function dismiss() {
    answered = true;
    writeDismissed();
  }
</script>

{#if visible}
  <div
    role="complementary"
    aria-label="Support freehire.me"
    class={cn(
      'fixed inset-x-4 bottom-4 z-30 rounded-lg border border-border bg-background/95 px-4 py-3 shadow-lg backdrop-blur sm:inset-x-auto sm:right-4 sm:max-w-md',
      // The job page anchors its Apply button to the same corner on the same layer below
      // `lg`. A promo never covers a page's own primary action, so here the toast waits
      // for the width at which that bar is gone.
      ownsMobileStickyCta(page.url.pathname) && 'hidden lg:block',
    )}
  >
    <div class="flex items-start gap-3">
      <!-- Two lines, not one wrapping paragraph. At phone width the single
           paragraph broke after "find", leaving "it." alone on the second line;
           stacking the sentences makes the break a layout decision instead of a
           consequence of the string's length. -->
      <div class="min-w-0 flex-1 text-sm">
        <p class="font-medium text-foreground">freehire.me is open source.</p>
        <p class="text-muted-foreground">Stars are how people find it.</p>
      </div>

      <button
        type="button"
        onclick={dismiss}
        aria-label="Dismiss support request"
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
