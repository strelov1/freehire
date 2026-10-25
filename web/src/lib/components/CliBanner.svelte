<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, X } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { dismissCliBanner, loadCliBannerDismissed, cliBannerDismissed } from '#lib/cliBanner.svelte.js';
  import { sourceLogoUrl } from '#lib/logo.js';
  import { isAccountArea } from '#lib/shellLayout.js';
  import { EntityLogo } from '#lib/ui/index.js';

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

  // The agents the strip names, shown as an overlapping stack of their marks (the same
  // avatar-stack CountryFlagStack draws for flags) instead of a sentence that wrapped to
  // four lines on a phone. Resolved by domain, as on /mcp, because "Cursor" by name
  // alone is ambiguous to the logo proxy.
  // Not inside the account area. Someone in /my/** has an account and came to work in it;
  // an MCP pitch there is one more band between the header and their page, and on a phone
  // that chrome had pushed the content halfway down the screen. Decided by the URL, which
  // the server knows too, so the server-rendered markup and the hydrated one agree.
  const inAccountArea = $derived(isAccountArea(page.url.pathname));

  // Gone for good once the account holds an API key or MCP grant. The user is resolved
  // in the root layout's server load, so the server leaves the strip out of the markup.
  const agentConnected = $derived(currentUser()?.agent_connected ?? false);

  const agents = [
    { name: 'Claude', domain: 'claude.ai' },
    { name: 'ChatGPT', domain: 'chatgpt.com' },
    { name: 'Cursor', domain: 'cursor.com' },
  ];
</script>

{#if !inAccountArea && !agentConnected && !cliBannerDismissed()}
  <div data-cli-banner class="border-b border-border bg-brand/5">
    <div class="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-2 text-sm">
      <!-- The names live in the stack's label, so the sentence can stay short. -->
      <span role="img" aria-label="Claude, ChatGPT and Cursor" class="flex shrink-0 items-center">
        {#each agents as agent, i (agent.domain)}
          <span
            class={['inline-flex rounded-full ring-2 ring-background', i > 0 && '-ml-1']}
            style:z-index={agents.length - i}
          >
            <EntityLogo name={agent.name} src={sourceLogoUrl(agent.name, agent.domain) ?? undefined} size="xs" />
          </span>
        {/each}
      </span>

      <!-- Wraps to a second line on a phone rather than truncating the one sentence that
           says what the strip is for. -->
      <p class="line-clamp-2 min-w-0 flex-1 leading-snug text-muted-foreground">
        <span class="font-medium text-foreground">Search jobs inside your AI agent.</span>
        <span class="hidden md:inline">Connect over MCP — free, no account needed to search.</span>
      </p>

      <a
        href={resolve('/mcp')}
        class="inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-brand px-3 py-1 text-xs font-medium text-brand-foreground transition-opacity hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
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
