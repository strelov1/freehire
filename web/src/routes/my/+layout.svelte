<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { User, LayoutList, Bell, Key, FileText } from '@lucide/svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { Button } from '$lib/ui';
  import { accountNav, isSectionActive, type AccountNavItem } from '$lib/accountNav';
  import { cn } from '$lib/utils';

  // The account shell: one source of truth for the `my/*` chrome — the width
  // container, the noindex tag, the auth gate, and the section navigation. Child
  // pages render only their own header + body inside the content column. The nav
  // is a vertical sidebar at lg+ and a horizontal scrollable strip below lg;
  // Tracking keeps its own sub-tabs (this is the top navigation level).

  let { children }: { children: Snippet } = $props();

  const path = $derived(page.url.pathname);

  // Icon per section, kept out of the pure accountNav model so that stays
  // Svelte-free and unit-testable. Keyed by the nav hrefs so a new section
  // without an icon is a compile error, not a runtime `<undefined />`.
  const icons: Record<AccountNavItem['href'], LucideIcon> = {
    '/my/profile': User,
    '/my/tracking': LayoutList,
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
    <!-- Same items, two forms; `extra` carries the per-form item tweaks. -->
    {#snippet navLinks(extra: string)}
      {#each accountNav as item (item.href)}
        {@const active = isSectionActive(path, item.href)}
        {@const Icon = icons[item.href]}
        <a
          href={resolve(item.href)}
          aria-current={active ? 'page' : undefined}
          class={cn(itemClass(active), extra)}
        >
          <Icon class="size-4 shrink-0" />
          {item.label}
        </a>
      {/each}
    {/snippet}

    <!-- Below lg: a horizontal, scrollable strip above the content. -->
    <nav aria-label="Account sections" class="mb-4 flex gap-1 overflow-x-auto lg:hidden">
      {@render navLinks('shrink-0 whitespace-nowrap')}
    </nav>

    <div class="lg:flex lg:gap-8">
      <!-- lg and up: a vertical sidebar beside the content. -->
      <aside aria-label="Account sections" class="hidden shrink-0 lg:block lg:w-56">
        <nav class="sticky top-6 flex flex-col gap-1">
          {@render navLinks('')}
        </nav>
      </aside>

      <div class="min-w-0 flex-1">
        {@render children()}
      </div>
    </div>
  {/if}
</div>
