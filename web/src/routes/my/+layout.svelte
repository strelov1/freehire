<script lang="ts">
  import type { Snippet } from 'svelte';
  import { setContext } from 'svelte';
  import { accountHeaderActions } from '#lib/accountHeader.js';
  import { onMount } from 'svelte';
  import { browser } from '$app/env';
  import { page } from '$app/state';
  import { PanelLeftClose, PanelLeft, ChevronDown, LifeBuoy } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';
  import { accountNav, activeSection } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import AccountNavDrawer from '#lib/components/AccountNavDrawer.svelte';
  import AccountNavGroups from '#lib/components/AccountNavGroups.svelte';
  import { messages as supportMessages } from '#lib/components/SupportPanel.messages.js';
  import SupportPanel from '#lib/components/SupportPanel.svelte';
  import { dockOffset } from '#lib/assistantDock.svelte.js';

  // The account shell: one source of truth for the `my/*` chrome — the width
  // container, the noindex tag, the auth gate, and the section navigation. Child
  // pages render only their own header + body inside the content column. The nav
  // is a collapsible vertical sidebar at lg+ and a slide-in drawer below lg (the
  // same one the agent's page opens); Tracking and Activity keep their own sub-tabs (this is the top level).

  let { children }: { children: Snippet } = $props();

  const s = $derived(t(messages, locale()));
  const path = $derived(page.url.pathname);

  // Collapsible sidebar (lg+), persisted so it stays across navigations/sessions.
  let collapsed = $state(false);
  onMount(() => {
    collapsed = localStorage.getItem('hire.myNavCollapsed') === '1';
  });
  function toggleNav() {
    collapsed = !collapsed;
    // An explicit choice outranks the dock's auto-collapse below, and survives it.
    restoreNavTo = null;
    if (browser) localStorage.setItem('hire.myNavCollapsed', collapsed ? '1' : '0');
  }

  // A docked assistant takes 360px off the right of the page. Collapsing the nav for its
  // duration buys back 168 of them, which is the difference between the bank keeping the
  // width it had and losing a fifth of it — see DOCKED_QUERY for the arithmetic.
  //
  // Never persisted: this is the dock borrowing the rail, not the candidate changing their
  // mind, so whatever they had chosen comes back when the panel closes.
  let restoreNavTo: boolean | null = null;
  $effect(() => {
    if (dockOffset() > 0) {
      if (restoreNavTo === null) {
        restoreNavTo = collapsed;
        collapsed = true;
      }
    } else if (restoreNavTo !== null) {
      collapsed = restoreNavTo;
      restoreNavTo = null;
    }
  });

  // Below lg the sections live in a drawer; the trigger names the section you are in,
  // so the page keeps saying where you are without a strip that scrolls off-screen.
  let drawerOpen = $state(false);
  const activeItem = $derived(activeSection(path, accountNav));

  setContext(accountHeaderActions, mobileActions);

  let helpOpen = $state(false);
  const helpLabel = $derived(t(supportMessages, locale()).title);
</script>

{#snippet mobileActions()}
  <div class="-mr-3 flex max-w-3/5 shrink-0 items-center gap-1 lg:hidden">
    <button
      type="button"
      onclick={() => (helpOpen = !helpOpen)}
      aria-haspopup="dialog"
      aria-expanded={helpOpen}
      title={helpLabel}
      class="flex min-h-8 w-fit items-center gap-2 rounded-md px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
    >
      <LifeBuoy class="size-4 shrink-0" aria-hidden="true" />
      <span class="hidden min-[390px]:inline">{helpLabel}</span>
    </button>
    <button
      type="button"
      onclick={() => (drawerOpen = true)}
      aria-haspopup="dialog"
      aria-expanded={drawerOpen}
      class="flex min-h-8 min-w-0 items-center gap-2 rounded-md px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent hover:text-accent-foreground lg:hidden"
    >
      <span class="sr-only">{s.shell.accountSections}:</span>
      {#if activeItem}
        {@const Icon = accountNavIcons[activeItem.href]}
        <Icon class="size-4 shrink-0" aria-hidden="true" />
        <span class="truncate">{navLabel(s, activeItem.href, activeItem.label)}</span>
      {:else}
        {s.rail.menu}
      {/if}
      <ChevronDown class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  </div>
{/snippet}

<svelte:head>
  <!-- Personal pages: keep the whole section out of search results, set once here. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<!-- The docked assistant is fixed to the viewport's right edge, so the shell steps aside
     rather than sharing its width with it. Padding on a wrapper, not a margin on the shell
     itself, so `mx-auto` keeps centring the content within whatever is left. -->
<div
  class="transition-[padding-right] duration-200 motion-reduce:transition-none"
  style:padding-right={dockOffset() ? `${dockOffset()}px` : undefined}
>
  <div class="mx-auto w-full max-w-6xl px-4 py-2 lg:py-6">
    <!-- No signed-out branch here: +layout.server.ts redirects an anonymous visitor to
         /signin before this renders, so the shell only ever draws for a session. -->

    <AccountNavDrawer bind:open={drawerOpen} />
    <SupportPanel open={helpOpen} onClose={() => (helpOpen = false)} />

    <div class="lg:flex lg:gap-8">
      <!-- lg and up: a collapsible vertical sidebar beside the content. A plain box, not
           an <aside>: a named complementary landmark wrapping a nav of the same name read
           as two "Account sections" landmarks, one inside the other. The nav is the
           landmark. -->
      <div class={cn('hidden shrink-0 transition-[width] duration-200 lg:block', collapsed ? 'lg:w-14' : 'lg:w-56')}>
        <!-- top-20 clears the sticky site header (h-14), the offset every other sticky
             side panel uses; at top-6 the collapse toggle sat behind it. Capped to the
             window and scrollable, so on a short window the last links stay reachable. -->
        <div class="sticky top-20 flex max-h-[calc(100vh-6.5rem)] flex-col gap-1 overflow-y-auto">
          <nav aria-label={s.shell.accountSections} class="flex flex-col gap-1">
            <AccountNavGroups variant="sidebar" iconOnly={collapsed} />
          </nav>
          <!-- Help and the collapse toggle close the column, so the nav's first item lines up
               with the section heading instead of under a row of utilities. -->
          <div
            class={cn(
              'mt-2 flex items-center border-t border-border pt-2',
              collapsed ? 'flex-col gap-1' : 'justify-between',
            )}
          >
            <button
              type="button"
              onclick={() => (helpOpen = !helpOpen)}
              aria-haspopup="dialog"
              aria-expanded={helpOpen}
              title={helpLabel}
              class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              <LifeBuoy class="size-4 shrink-0" aria-hidden="true" />
              <span class={collapsed ? 'sr-only' : undefined}>{helpLabel}</span>
            </button>
            <button
              type="button"
              onclick={toggleNav}
              title={collapsed ? s.shell.expandSidebar : s.shell.collapseSidebar}
              aria-label={collapsed ? s.shell.expandSidebar : s.shell.collapseSidebar}
              class="flex items-center rounded-md px-3 py-2 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            >
              {#if collapsed}
                <PanelLeft class="size-4" />
              {:else}
                <PanelLeftClose class="size-4" />
              {/if}
            </button>
          </div>
        </div>
      </div>

      <div class="min-w-0 flex-1">
        {@render children()}
      </div>
    </div>
  </div>
</div>
