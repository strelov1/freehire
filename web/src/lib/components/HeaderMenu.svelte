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
  const sectionLabel =
    'px-4 pt-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground sm:hidden';

  // The theme button only renders inside the open panel (client-only), so no
  // SSR mounted-guard is needed — read the store directly.
  const isDark = $derived(themeStore.isDark);
  const email = $derived(currentUser()?.email ?? '');
  const isModerator = $derived(currentUser()?.role === 'moderator');

  // Static nav (always shown) and the signed-in account items (shown only when
  // authenticated). Moderation is gated on the moderator role at render time.
  const navLinks = [
    { href: '/jobs', label: 'Jobs' },
    { href: '/companies', label: 'Companies' },
    { href: '/collections', label: 'Collections' },
    { href: '/analytics', label: 'Analytics' },
    { href: '/cli', label: 'CLI' },
    { href: '/recruiters', label: 'For recruiters' },
    { href: '/for-companies', label: 'For companies' },
  ] as const;

  const accountLinks = [
    { href: '/my/jobs', label: 'My jobs' },
    { href: '/my/profiles', label: 'Profiles' },
    { href: '/my/searches', label: 'Saved searches' },
    { href: '/my/notifications', label: 'Notifications' },
    { href: '/my/api-keys', label: 'API keys' },
    { href: '/submit', label: 'Submit a job' },
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

<!-- Theme toggle + auth action: defined once, rendered in the mobile bottom bar
     and inline on desktop. -->
{#snippet themeAuth()}
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

<div class="relative" bind:this={root}>
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
    class="inline-flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
          <svg viewBox="0 0 512 512" class="size-5 shrink-0" fill="currentColor" aria-hidden="true">
            <path
              fill-rule="evenodd"
              clip-rule="evenodd"
              d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"
            />
          </svg>
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
        {#if isAuthenticated()}
          <p class="truncate px-4 py-2 text-sm text-muted-foreground sm:px-3" title={email}>{email}</p>
          <div class="my-1 hidden h-px bg-border sm:block"></div>
        {/if}

        <!-- Nav -->
        <p class={sectionLabel}>Navigate</p>
        {#each navLinks as link (link.href)}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            {link.label}
          </a>
        {/each}

        <!-- Account -->
        {#if isAuthenticated()}
          <div class="my-1 hidden h-px bg-border sm:block"></div>
          <p class={sectionLabel}>Account</p>
          {#each accountLinks as link (link.href)}
            <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
              {link.label}
            </a>
          {/each}
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
        {/if}

        <!-- Desktop-only: theme + auth inline at the end of the dropdown. -->
        <div class="hidden sm:block">
          <div class="my-1 h-px bg-border"></div>
          {@render themeAuth()}
        </div>
      </div>

      <!-- Mobile-only: theme + auth pinned to the bottom of the drawer. -->
      <div class="shrink-0 border-t border-border p-2 sm:hidden">
        {@render themeAuth()}
      </div>
    </div>
  {/if}
</div>
