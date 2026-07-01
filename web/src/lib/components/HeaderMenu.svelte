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

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const path = $derived(page.url.pathname);
  const isActive = (href: string) => path === href || path.startsWith(`${href}/`);

  // Shared classes for every menu link; the active section is emphasised.
  const linkClass = (href: string) =>
    cn(
      'block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground',
      isActive(href) ? 'font-medium text-foreground' : 'text-muted-foreground',
    );
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
    { href: '/my/profiles', label: 'Search profiles' },
    { href: '/my/searches', label: 'Saved searches' },
    { href: '/my/notifications', label: 'Notifications' },
    { href: '/my/api-keys', label: 'API keys' },
    { href: '/submit', label: 'Submit a job' },
    { href: '/my/submissions', label: 'My submissions' },
  ] as const;

  // Mobile only: the open panel is a full-width overlay, so lock the page behind
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
    <!-- Mobile backdrop behind the full-width panel. -->
    <button
      class="fixed inset-0 z-40 bg-black/40 sm:hidden"
      aria-label="Close menu"
      onclick={() => (open = false)}
    ></button>

    <div
      role="menu"
      class="z-50 max-h-[80vh] overflow-y-auto border border-border bg-background py-1 shadow-lg
             max-sm:fixed max-sm:inset-x-0 max-sm:top-14 max-sm:mt-0 max-sm:rounded-none max-sm:border-x-0
             sm:absolute sm:right-0 sm:top-full sm:mt-2 sm:w-64 sm:rounded-md"
    >
      {#if isAuthenticated()}
        <p class="truncate px-3 py-2 text-sm text-muted-foreground" title={email}>{email}</p>
        <div class="my-1 h-px bg-border"></div>
      {/if}

      <!-- Nav -->
      {#each navLinks as link (link.href)}
        <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
          {link.label}
        </a>
      {/each}

      <!-- Account -->
      {#if isAuthenticated()}
        <div class="my-1 h-px bg-border"></div>
        {#each accountLinks as link (link.href)}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            {link.label}
          </a>
        {/each}
        {#if isModerator}
          <a href={resolve('/moderation')} role="menuitem" onclick={() => (open = false)} class={linkClass('/moderation')}>
            Moderation
          </a>
        {/if}
      {/if}

      <!-- Theme + auth -->
      <div class="my-1 h-px bg-border"></div>
      <button
        type="button"
        role="menuitem"
        onclick={() => themeStore.toggle()}
        class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        {#if isDark}
          <Moon class="size-4" /> Light theme
        {:else}
          <Sun class="size-4" /> Dark theme
        {/if}
      </button>

      {#if isAuthenticated()}
        <button
          type="button"
          role="menuitem"
          onclick={logout}
          class="block w-full px-3 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          Log out
        </button>
      {:else}
        <button
          type="button"
          role="menuitem"
          onclick={signIn}
          class="block w-full px-3 py-2 text-left text-sm font-medium text-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          Sign in
        </button>
      {/if}
    </div>
  {/if}
</div>
