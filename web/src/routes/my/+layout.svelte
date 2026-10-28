<script lang="ts">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { browser } from '$app/env';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { PanelLeftClose, PanelLeft, ChevronDown } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';
  import { visibleAccountNav, isSectionActive } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import TalentNetworkInvite from '#lib/components/TalentNetworkInvite.svelte';
  import AccountNavDrawer from '#lib/components/AccountNavDrawer.svelte';
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

  // Sections marked moderator- or beta-only are hidden from the nav for everyone
  // else; the relevant server surfaces enforce the real gate, this just declutters
  // the UI. No section carries such a mark today — the Agent lost its beta gate and
  // is open to every signed-in user.
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );

  // Below lg the sections live in a drawer; the trigger names the section you are in,
  // so the page keeps saying where you are without a strip that scrolls off-screen.
  let drawerOpen = $state(false);
  const activeItem = $derived(navItems.find((i) => isSectionActive(path, i.href)));

  // Sidebar item treatment; active matches the tracking tabs.
  const itemClass = (active: boolean) =>
    cn(
      'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
      active
        ? 'bg-secondary font-medium text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    );
</script>

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
  <div class="mx-auto w-full max-w-6xl px-4 py-3 lg:py-6">
    <!-- No signed-out branch here: +layout.server.ts redirects an anonymous visitor to
         /signin before this renders, so the shell only ever draws for a session. -->

    <!-- When `iconOnly`, the label is dropped and the icon centres (collapsed rail). -->
    {#snippet navLinks(iconOnly: boolean)}
      {#each navItems as item (item.href)}
        {@const active = isSectionActive(path, item.href)}
        {@const Icon = accountNavIcons[item.href]}
        {@const label = navLabel(s, item.href, item.label)}
        <a
          href={resolve(toPath(item.href))}
          aria-current={active ? 'page' : undefined}
          title={iconOnly ? label : undefined}
          class={cn(itemClass(active), iconOnly && 'justify-center')}
        >
          <Icon class="size-4 shrink-0" />
          {#if !iconOnly}
            {label}
          {/if}
        </a>
      {/each}
    {/snippet}

    <!-- Below lg: a section switcher naming where you are, opening the drawer. A chevron,
         not a burger: the site header already carries one, and two identical icons a row
         apart read as the same control. The accessible name keeps the visible label in it
         (WCAG 2.5.3), prefixed so a screen reader hears what the button switches. The
         negative margin lines the section icon up with the content below.

         It is also the section's visible title below lg: each section's own heading and
         description carry `max-lg:sr-only`, since repeating the name this button already
         shows only pushed the content down on a phone. -->
    <button
      type="button"
      onclick={() => (drawerOpen = true)}
      aria-haspopup="dialog"
      aria-expanded={drawerOpen}
      class="-ml-3 mb-2 flex min-h-11 items-center gap-2 rounded-md px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent hover:text-accent-foreground lg:hidden"
    >
      <span class="sr-only">{s.shell.accountSections}:</span>
      {#if activeItem}
        {@const Icon = accountNavIcons[activeItem.href]}
        <Icon class="size-4 shrink-0" aria-hidden="true" />
        {navLabel(s, activeItem.href, activeItem.label)}
      {:else}
        {s.rail.menu}
      {/if}
      <ChevronDown class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
    <AccountNavDrawer bind:open={drawerOpen} />

    <div class="lg:flex lg:gap-8">
      <!-- lg and up: a collapsible vertical sidebar beside the content. A plain box, not
           an <aside>: a named complementary landmark wrapping a nav of the same name read
           as two "Account sections" landmarks, one inside the other. The nav is the
           landmark. -->
      <div
        class={cn(
          'hidden shrink-0 transition-[width] duration-200 lg:block',
          collapsed ? 'lg:w-14' : 'lg:w-56',
        )}
      >
        <div class="sticky top-6 flex flex-col gap-1">
          <button
            type="button"
            onclick={toggleNav}
            title={collapsed ? s.shell.expandSidebar : s.shell.collapseSidebar}
            aria-label={collapsed ? s.shell.expandSidebar : s.shell.collapseSidebar}
            class={cn(
              'mb-1 flex items-center rounded-md px-3 py-2 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
              collapsed && 'justify-center',
            )}
          >
            {#if collapsed}
              <PanelLeft class="size-4" />
            {:else}
              <PanelLeftClose class="size-4" />
            {/if}
          </button>
          <nav aria-label={s.shell.accountSections} class="flex flex-col gap-1">
            {@render navLinks(collapsed)}
          </nav>
        </div>
      </div>

      <div class="min-w-0 flex-1">
        <!-- Above the section's own heading, and above every section rather than only
             Profile's — the card itself argues why, and collapses to a single row on a
             phone so it does not push that heading down. All this layer decides is the
             one page it must not appear on: a banner pointing at the page you are reading
             is noise. Dismissal is the card's own business. -->
        {#if !isSectionActive(path, '/my/talent-network')}
          <TalentNetworkInvite />
        {/if}

        {@render children()}
      </div>
    </div>
  </div>
</div>
