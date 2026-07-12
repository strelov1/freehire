<script lang="ts">
  import { page } from '$app/state';
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Menu, X, Sun, Moon } from '@lucide/svelte';
  import { isAuthenticated, currentUser, logout as doLogout } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { themeStore } from '$lib/theme.svelte';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { cn } from '$lib/utils';
  import BrandMark from './BrandMark.svelte';
  import GithubStars from './GithubStars.svelte';

  // The single menu absorbs the site nav, the signed-in account items, the theme
  // toggle, and the auth action — the header's only control besides search.
  //
  // Two layouts from one markup: on mobile the panel is a full-screen drawer
  // (own top bar · scrollable sectioned links · pinned bottom action bar); on
  // desktop it stays the small anchored dropdown. The theme+auth actions live in
  // one snippet, rendered in the mobile bottom bar and inline for desktop.

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const path = $derived(page.url.pathname);
  const isActive = (href: string) => path === href || path.startsWith(`${href}/`);

  // Shared row classes: a ≥44px tap target with base text on mobile, collapsing
  // to the compact dropdown row on desktop. Active section is emphasised.
  const rowBase =
    'flex items-center gap-2 rounded-md px-4 min-h-11 text-base transition-colors hover:bg-accent hover:text-accent-foreground sm:min-h-0 sm:rounded-none sm:px-3 sm:py-2 sm:text-sm';
  const linkClass = (href: string) =>
    cn(rowBase, isActive(href) ? 'font-medium text-foreground' : 'text-muted-foreground');
  // Shared icon-button treatment for the bar controls (menu + theme toggle).
  const iconButton =
    'size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  // The theme button only renders inside the open panel (client-only), so no
  // SSR mounted-guard is needed — read the store directly.
  const isDark = $derived(themeStore.isDark);
  const email = $derived(currentUser()?.email ?? '');
  const isModerator = $derived(currentUser()?.role === 'moderator');

  // Static nav (always shown) and the signed-in account items (shown only when
  // authenticated). Moderation is gated on the moderator role at render time.
  // Primary destinations pinned to the very top of the menu. Jobs is the homepage
  // feed (also reachable via the logo); Companies leads the browse surfaces.
  const primaryLinks = [
    { href: '/', label: 'Jobs' },
    { href: '/companies', label: 'Companies' },
  ] as const;

  const navLinks = [
    { href: '/collections', label: 'Collections' },
    { href: '/analytics', label: 'Analytics' },
    { href: '/trends', label: 'Trends' },
  ] as const;

  // Personal account items — what the signed-in user owns/reads. "Submit a job"
  // is a create action, so it's rendered separately (below), split off from the
  // "My submissions" reading item it used to sit next to.
  const accountLinks = [
    { href: '/my/tracking', label: 'Tracking' },
    { href: '/my/searches', label: 'Search notifications' },
    { href: '/my/api-keys', label: 'API keys' },
    { href: '/my/submissions', label: 'My submissions' },
  ] as const;

  // Mobile only: the open panel is a full-screen overlay, so lock the page behind
  // it. Desktop keeps the small anchored dropdown and stays scrollable.
  $effect(() => {
    if (!open) return;
    if (window.matchMedia('(min-width: 640px)').matches) return;
    lockScroll();
    return () => unlockScroll();
  });

  afterNavigate(() => {
    open = false;
  });

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  function signIn() {
    open = false;
    openAuthDialog('login');
  }

  function logout() {
    open = false;
    void doLogout();
  }
</script>

<svelte:window
  onclick={onWindowClick}
  onkeydown={(e) => e.key === 'Escape' && (open = false)}
/>

<!-- Theme toggle and auth action: defined once, reused across layouts. On mobile
     both live deep in the drawer's bottom bar; on desktop the theme toggle is
     pulled up to the header bar (see the inline cluster below) and only auth
     stays in the dropdown. -->
{#snippet themeButton()}
  <button
    type="button"
    role="menuitem"
    onclick={() => themeStore.toggle()}
    class={cn(rowBase, 'w-full text-left text-muted-foreground')}
  >
    {#if isDark}
      <Moon class="size-4 shrink-0" /> Light theme
    {:else}
      <Sun class="size-4 shrink-0" /> Dark theme
    {/if}
  </button>
{/snippet}

{#snippet authButton()}
  {#if isAuthenticated()}
    <button
      type="button"
      role="menuitem"
      onclick={logout}
      class={cn(rowBase, 'w-full text-left text-muted-foreground')}
    >
      Log out
    </button>
  {:else}
    <button
      type="button"
      role="menuitem"
      onclick={signIn}
      class={cn(rowBase, 'w-full text-left font-medium text-foreground')}
    >
      Sign in
    </button>
  {/if}
{/snippet}

<div class="relative flex items-center gap-1" bind:this={root}>
  <!-- Desktop bar order: GitHub stars, then the theme toggle (second), then the
       menu button pinned to the far right. On mobile both GitHub and theme collapse
       into the drawer (below), leaving just the menu button here. -->
  <GithubStars class="hidden sm:inline-flex" />

  <!-- Desktop only: theme toggle sits second, before the menu button. -->
  <button
    type="button"
    aria-label={isDark ? 'Switch to light theme' : 'Switch to dark theme'}
    onclick={() => themeStore.toggle()}
    class={cn('hidden sm:inline-flex', iconButton)}
  >
    {#if isDark}
      <Moon class="size-5" />
    {:else}
      <Sun class="size-5" />
    {/if}
  </button>

  <button
    type="button"
    aria-label="Menu"
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={(e) => {
      // Stop the toggle's own click from reaching the window outside-handler.
      // Without this, opening detaches the clicked icon (the {#if open} Menu/X
      // swap) from the DOM, so onWindowClick's root.contains(e.target) reads
      // false and immediately re-closes the just-opened menu — the "center of
      // the button doesn't open, only the edge does" bug.
      e.stopPropagation();
      open = !open;
    }}
    class={cn('inline-flex', iconButton)}
  >
    {#if open}
      <X class="size-5" />
    {:else}
      <Menu class="size-5" />
    {/if}
  </button>

  {#if open}
    <div
      role="menu"
      class="z-50 bg-background
             max-sm:fixed max-sm:inset-0 max-sm:flex max-sm:flex-col
             sm:absolute sm:right-0 sm:top-full sm:mt-2 sm:max-h-[80vh] sm:w-64 sm:overflow-y-auto sm:rounded-md sm:border sm:border-border sm:py-1 sm:shadow-lg"
    >
      <!-- Mobile top bar: brand + close (drawer reads as a screen, not a dropdown). -->
      <div class="flex h-14 shrink-0 items-center justify-between border-b border-border px-4 sm:hidden">
        <span class="flex items-center gap-2 text-sm font-semibold tracking-tight">
          <BrandMark />
          FreeHire
        </span>
        <button
          type="button"
          aria-label="Close menu"
          onclick={() => (open = false)}
          class="inline-flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          <X class="size-5" />
        </button>
      </div>

      <!-- Middle: scrollable, sectioned links. -->
      <div class="max-sm:flex-1 max-sm:overflow-y-auto max-sm:px-2 max-sm:pb-3">
        <!-- Primary destinations pinned to the top, then a divider before the
             signed-in personal items and the rest of the site nav. -->
        {#each primaryLinks as link (link.href)}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            {link.label}
          </a>
        {/each}
        <div class="my-1 h-px bg-border"></div>

        {#if isAuthenticated()}
          <a
            href={resolve('/my/profile')}
            role="menuitem"
            onclick={() => (open = false)}
            class={linkClass('/my/profile')}
            title={email}
          >
            Profile
          </a>
          {#each accountLinks as link (link.href)}
            <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
              {link.label}
            </a>
          {/each}

          <!-- Create/action items, split off from the account reading items above. -->
          <div class="my-1 h-px bg-border"></div>
          <a href={resolve('/submit')} role="menuitem" onclick={() => (open = false)} class={linkClass('/submit')}>
            Submit a job
          </a>
          {#if isModerator}
            <a
              href={resolve('/moderation')}
              role="menuitem"
              onclick={() => (open = false)}
              class={linkClass('/moderation')}
            >
              Moderation
            </a>
          {/if}
          <div class="my-1 h-px bg-border"></div>
        {/if}

        {#each navLinks as link (link.href)}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            {link.label}
          </a>
        {/each}

        <!-- About sits at the very bottom of the link list, just before the
             Sign in / Log out action (the marketing landing lives at /about). -->
        <a href={resolve('/about')} role="menuitem" onclick={() => (open = false)} class={linkClass('/about')}>
          About
        </a>

        <!-- Desktop-only: auth inline at the end of the dropdown (theme lives on
             the bar). -->
        <div class="hidden sm:block">
          <div class="my-1 h-px bg-border"></div>
          {@render authButton()}
        </div>
      </div>

      <!-- Mobile-only: GitHub + theme + auth pinned to the bottom of the drawer. -->
      <div class="shrink-0 border-t border-border p-2 sm:hidden">
        <GithubStars variant="row" class={cn(rowBase, 'text-muted-foreground')} />
        {@render themeButton()}
        {@render authButton()}
      </div>
    </div>
  {/if}
</div>
