<script lang="ts">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import {
    User,
    Bot,
    LayoutList,
    Activity,
    Bell,
    Key,
    FileText,
    Inbox,
    PanelLeftClose,
    PanelLeft,
  } from '@lucide/svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { isAuthenticated, currentUser } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { Button } from '$lib/ui';
  import { visibleAccountNav, isSectionActive, type AccountNavItem } from '$lib/accountNav';
  import { cn } from '$lib/utils';

  // The account shell: one source of truth for the `my/*` chrome — the width
  // container, the noindex tag, the auth gate, and the section navigation. Child
  // pages render only their own header + body inside the content column. The nav
  // is a collapsible vertical sidebar at lg+ and a horizontal scrollable strip
  // below lg; Tracking and Activity keep their own sub-tabs (this is the top level).

  let { children }: { children: Snippet } = $props();

  const path = $derived(page.url.pathname);

  // Collapsible sidebar (lg+), persisted so it stays across navigations/sessions.
  let collapsed = $state(false);
  onMount(() => {
    collapsed = localStorage.getItem('hire.myNavCollapsed') === '1';
  });
  function toggleNav() {
    collapsed = !collapsed;
    if (browser) localStorage.setItem('hire.myNavCollapsed', collapsed ? '1' : '0');
  }

  // Moderator-only sections (Inbox) and beta-only sections (Agent) are hidden
  // from the nav for everyone else; the relevant server surfaces enforce the
  // real gate, this just declutters the UI.
  const navItems = $derived(
    visibleAccountNav(currentUser()?.role === 'moderator', currentUser()?.beta_tester ?? false),
  );

  // Icon per section, kept out of the pure accountNav model so that stays
  // Svelte-free and unit-testable. Keyed by the nav hrefs so a new section
  // without an icon is a compile error, not a runtime `<undefined />`.
  const icons: Record<AccountNavItem['href'], LucideIcon> = {
    '/my/profile': User,
    '/my/assistant': Bot,
    '/my/tracking': LayoutList,
    '/my/activity': Activity,
    '/my/inbox': Inbox,
    '/my/searches': Bell,
    '/my/api-keys': Key,
    '/my/submissions': FileText,
  };

  // Shared item treatment for both nav forms; active matches the tracking tabs.
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

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to access your account.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else}
    <!-- Same items, two forms; `extra` carries the per-form item tweaks. When
         `iconOnly`, the label is dropped and the icon centres (collapsed rail). -->
    {#snippet navLinks(extra: string, iconOnly = false)}
      {#each navItems as item (item.href)}
        {@const active = isSectionActive(path, item.href)}
        {@const Icon = icons[item.href]}
        <a
          href={resolve(item.href)}
          aria-current={active ? 'page' : undefined}
          title={iconOnly ? item.label : undefined}
          class={cn(itemClass(active), iconOnly && 'justify-center', extra)}
        >
          <Icon class="size-4 shrink-0" />
          {#if !iconOnly}
            {item.label}
            {#if 'beta' in item && item.beta}
              <span
                class="ml-auto rounded-full bg-brand-muted px-1.5 py-0.5 text-[10px] font-medium uppercase leading-none tracking-wide text-brand-strong"
              >
                Beta
              </span>
            {/if}
          {/if}
        </a>
      {/each}
    {/snippet}

    <!-- Below lg: a horizontal, scrollable strip above the content. -->
    <nav aria-label="Account sections" class="mb-4 flex gap-1 overflow-x-auto lg:hidden">
      {@render navLinks('shrink-0 whitespace-nowrap')}
    </nav>

    <div class="lg:flex lg:gap-8">
      <!-- lg and up: a collapsible vertical sidebar beside the content. -->
      <aside
        aria-label="Account sections"
        class={cn('hidden shrink-0 transition-[width] duration-200 lg:block', collapsed ? 'lg:w-14' : 'lg:w-56')}
      >
        <div class="sticky top-6 flex flex-col gap-1">
          <button
            type="button"
            onclick={toggleNav}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
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
          <nav aria-label="Account sections" class="flex flex-col gap-1">
            {@render navLinks('', collapsed)}
          </nav>
        </div>
      </aside>

      <div class="min-w-0 flex-1">
        {@render children()}
      </div>
    </div>
  {/if}
</div>
