<script lang="ts">
  import { onMount } from 'svelte';
  import { launchPhase, PH_URL, readDismissed, writeDismissed } from '$lib/productHunt';

  // A strip under the header pointing at the Product Hunt page, dismissible and
  // remembered. Self-gating like EmailVerificationBanner, so the layout mounts it
  // unconditionally: it renders nothing once the launch day has passed.
  //
  // Its wording follows the clock (`launchPhase`) rather than a flag, so it turns
  // itself from "launching on the 26th" into "live today" and then retires — nobody
  // has to ship a web build on launch day.
  //
  // Why it renders on the server and hides via a class rather than waiting for mount:
  // this strip sits in the document flow, so mounting it after hydration would shove
  // the page down under someone already reading it. The no-flash script in app.html
  // sets `.ph-dismissed` before first paint for a visitor who closed it, exactly as it
  // does for the theme; `dismissed` then catches up on mount and removes the node.

  const phase = launchPhase(Date.now());

  // Starts false so the hydrated markup matches the server's; the real value arrives
  // on mount, by which point CSS has already hidden the strip for anyone who closed it.
  let dismissed = $state(false);

  onMount(() => {
    dismissed = readDismissed();
  });

  function dismiss() {
    dismissed = true;
    writeDismissed();
  }
</script>

{#if phase !== 'over' && !dismissed}
  <div data-ph-banner class="border-b border-border bg-brand/5">
    <div class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-2.5 text-sm">
      <p class="min-w-0 flex-1 text-muted-foreground">
        {#if phase === 'live'}
          <span class="font-medium text-foreground">freehire is live on Product Hunt today.</span>
          <span class="hidden sm:inline">One day decides whether anyone sees it.</span>
        {:else}
          <span class="font-medium text-foreground"
            >freehire launches on Product Hunt on 26 August.</span
          >
          <span class="hidden sm:inline">Follow the page and you'll hear the moment it opens.</span>
        {/if}
      </p>

      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Product Hunt page opened in a new tab; not an internal route -->
      <a
        href={PH_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="shrink-0 whitespace-nowrap font-medium text-brand underline-offset-4 hover:underline"
      >
        {phase === 'live' ? 'Support the launch' : 'Follow'} →
      </a>

      <button
        type="button"
        onclick={dismiss}
        aria-label="Dismiss"
        class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
      >
        ✕
      </button>
    </div>
  </div>
{/if}
