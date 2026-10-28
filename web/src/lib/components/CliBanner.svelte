<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, X } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { dismissCliBanner, loadCliBannerDismissed, cliBannerDismissed } from '#lib/cliBanner.svelte.js';
  import { isAccountArea } from '#lib/shellLayout.js';
  import { buttonVariants, cn } from '#lib/ui/index.js';

  // A strip under the header pointing at /mcp (it pointed at /cli until the hosted MCP
  // connectors became the main way in). It asks until the account connects an agent;
  // closing it only quiets it for the browser session (see #lib/cliPromo.ts). Self-gating
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

  // The agents the strip names — Claude, ChatGPT, Gemini, Cursor and Grok — drawn as one
  // animated stack in /brands/agent-logos.svg instead of a sentence that wrapped to four
  // lines on a phone. One file rather than four proxied logos: the marks are fixed brand
  // art, and the lift animation runs inside the SVG with no script.

  // Not inside the account area. Someone in /my/** has an account and came to work in it;
  // an MCP pitch there is one more band between the header and their page, and on a phone
  // that chrome had pushed the content halfway down the screen. Decided by the URL, which
  // the server knows too, so the server-rendered markup and the hydrated one agree.
  const inAccountArea = $derived(isAccountArea(page.url.pathname));

  // Gone for good once the account holds an API key or MCP grant. The user is resolved
  // in the root layout's server load, so the server leaves the strip out of the markup.
  const agentConnected = $derived(currentUser()?.agent_connected ?? false);
</script>

{#if !inAccountArea && !agentConnected && !cliBannerDismissed()}
  <div data-cli-banner class="border-b border-border bg-brand/5">
    <div class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-2 text-sm">
      <!-- Wraps to a second line on a phone rather than truncating the one sentence that
           says what the strip is for. The agents' marks follow the sentence that names
           them; their names live in the stack's label, so the sentence can stay short. -->
      <p class="line-clamp-2 min-w-0 flex-1 leading-snug text-muted-foreground">
        <span class="font-medium text-foreground">Search jobs inside your AI agent.</span>
        <img
          src="/brands/agent-logos.svg"
          alt="Claude, ChatGPT, Gemini, Cursor and Grok"
          width="161"
          height="53"
          class="mx-1 -my-1 inline-block h-7 w-auto align-middle"
        />
        <span class="hidden md:inline">Connect over MCP — free, no account needed to search.</span>
      </p>

      <a
        href={resolve('/mcp')}
        class={cn(buttonVariants({ variant: 'cta', size: 'sm' }), 'h-7 shrink-0 gap-1 rounded-full px-3 text-xs')}
      >
        Connect
        <ArrowRight class="size-3.5" aria-hidden="true" />
      </a>

      <button
        type="button"
        onclick={dismissCliBanner}
        aria-label="Dismiss"
        class="relative flex shrink-0 items-center justify-center text-muted-foreground transition-colors after:absolute after:-inset-1 hover:text-foreground"
      >
        <X class="size-4" aria-hidden="true" />
      </button>
    </div>
  </div>
{/if}
