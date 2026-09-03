<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { dismissCliBanner, loadCliBannerDismissed, cliBannerDismissed } from '$lib/cliBanner.svelte';

  // A strip under the header pointing at /cli, dismissible and remembered. Self-gating
  // like EmailVerificationBanner, so the layout mounts it unconditionally.
  //
  // It took the place of the Product Hunt launch strip and kept its arrangement, which
  // was the part worth keeping. Why it renders on the server and hides via a class
  // rather than waiting for mount: this strip sits in the document flow, so mounting it
  // after hydration would shove the page down under someone already reading it. The
  // no-flash script in app.html sets `.cli-banner-dismissed` before first paint for a
  // visitor who closed it, exactly as it does for the theme; `dismissed` then catches up
  // on mount and removes the node.

  // The flag starts false so the hydrated markup matches the server's; the real value
  // arrives on mount, by which point CSS has already hidden the strip for anyone who
  // closed it.
  onMount(() => {
    loadCliBannerDismissed();
  });
</script>

{#if !cliBannerDismissed()}
  <div data-cli-banner class="border-b border-border bg-brand/5">
    <div class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-2.5 text-sm">
      <p class="min-w-0 flex-1 text-muted-foreground">
        <span class="font-medium text-foreground">
          Point your AI agent at freehire and let it find you a job.
        </span>
        <span class="hidden sm:inline">
          A CLI and an MCP server over the whole job API — no browser.
        </span>
      </p>

      <a
        href={resolve('/cli')}
        class="shrink-0 whitespace-nowrap font-medium text-brand underline-offset-4 hover:underline"
      >
        Get the CLI →
      </a>

      <button
        type="button"
        onclick={dismissCliBanner}
        aria-label="Dismiss"
        class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
      >
        ✕
      </button>
    </div>
  </div>
{/if}
