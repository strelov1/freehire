<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { Menu, X } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { authDialog, openAuthDialog, closeAuthDialog } from '$lib/auth-dialog.svelte';
  import { cn } from '$lib/utils';
  import { Button } from '$lib/ui';
  import ThemeToggle from './ThemeToggle.svelte';
  import AuthDialog from './AuthDialog.svelte';
  import UserMenu from './UserMenu.svelte';

  const path = $derived(page.url.pathname);

  // The nav links collapse behind a hamburger below the `sm` breakpoint, where
  // five inline links would overflow the bar. The panel closes on link tap,
  // Escape, and whenever the route changes.
  let mobileOpen = $state(false);

  const links = [
    { href: '/jobs', label: 'Jobs' },
    { href: '/companies', label: 'Companies' },
    { href: '/analytics', label: 'Analytics' },
    { href: '/cli', label: 'CLI' },
    { href: '/recruiters', label: 'For recruiters' },
  ];

  // A link is active for its own path and any sub-route (e.g. /jobs and
  // /jobs/:slug). Order-independent: /companies never matches /jobs.
  const isActive = (href: string) => path === href || path.startsWith(`${href}/`);

  // The auth dialog lives at the layout level but its open state is a shared
  // singleton (see auth-dialog.svelte), so deep components — like a job's Save
  // button — can prompt sign-in through the same dialog this header renders.

  // Surface a failed OAuth callback (?auth_error) once on the client, then clean
  // the URL. In onMount so it never runs during SSR.
  onMount(() => {
    if (page.url.searchParams.has('auth_error')) {
      openAuthDialog('login', 'Sign-in failed. Please try again.');
      replaceState(page.url.pathname, {});
    }
  });
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && (mobileOpen = false)} />

<header class="border-b border-border">
  <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:gap-6">
    <a href="/" class="text-sm font-semibold tracking-tight">FreeHire</a>

    <!-- Desktop nav: inline links from `sm` up. -->
    <nav class="hidden items-center gap-4 text-sm sm:flex">
      {#each links as link (link.href)}
        <a
          href={link.href}
          class={cn(
            'whitespace-nowrap transition-colors hover:text-foreground',
            isActive(link.href) ? 'text-foreground' : 'text-muted-foreground',
          )}
        >
          {link.label}
        </a>
      {/each}
    </nav>

    <div class="ml-auto flex items-center gap-2 sm:gap-3">
      {#if isAuthenticated()}
        <UserMenu />
      {:else}
        <Button variant="primary" size="sm" onclick={() => openAuthDialog('login')}>Sign in</Button>
      {/if}
      <ThemeToggle />

      <!-- Mobile nav toggle: only below `sm`. -->
      <button
        type="button"
        onclick={() => (mobileOpen = !mobileOpen)}
        aria-label="Toggle navigation menu"
        aria-expanded={mobileOpen}
        class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground sm:hidden"
      >
        {#if mobileOpen}
          <X class="size-5" />
        {:else}
          <Menu class="size-5" />
        {/if}
      </button>
    </div>
  </div>

  <!-- Mobile nav panel: stacked links, shown when the hamburger is open. -->
  {#if mobileOpen}
    <nav class="flex flex-col border-t border-border px-4 py-1 sm:hidden">
      {#each links as link (link.href)}
        <a
          href={link.href}
          onclick={() => (mobileOpen = false)}
          class={cn(
            'rounded-md px-2 py-2.5 text-sm transition-colors hover:bg-accent hover:text-accent-foreground',
            isActive(link.href) ? 'text-foreground' : 'text-muted-foreground',
          )}
        >
          {link.label}
        </a>
      {/each}
    </nav>
  {/if}
</header>

{#if authDialog.open}
  <AuthDialog bind:mode={authDialog.mode} initialError={authDialog.error} onClose={closeAuthDialog} />
{/if}
