<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { authDialog, openAuthDialog, closeAuthDialog } from '$lib/auth-dialog.svelte';
  import { cn } from '$lib/utils';
  import { Button } from '$lib/ui';
  import ThemeToggle from './ThemeToggle.svelte';
  import AuthDialog from './AuthDialog.svelte';
  import UserMenu from './UserMenu.svelte';

  const path = $derived(page.url.pathname);

  const links = [
    { href: '/jobs', label: 'Jobs' },
    { href: '/companies', label: 'Companies' },
    { href: '/analytics', label: 'Analytics' },
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

<header class="border-b border-border">
  <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:gap-6">
    <a href="/" class="text-sm font-semibold tracking-tight">FreeHire</a>

    <nav class="flex items-center gap-4 text-sm">
      {#each links as link (link.href)}
        <a
          href={link.href}
          class={cn(
            'transition-colors hover:text-foreground',
            isActive(link.href) ? 'text-foreground' : 'text-muted-foreground',
          )}
        >
          {link.label}
        </a>
      {/each}
    </nav>

    <div class="ml-auto flex items-center gap-3">
      {#if isAuthenticated()}
        <UserMenu />
      {:else}
        <Button variant="primary" size="sm" onclick={() => openAuthDialog('login')}>Sign in</Button>
      {/if}
      <ThemeToggle />
    </div>
  </div>
</header>

{#if authDialog.open}
  <AuthDialog bind:mode={authDialog.mode} initialError={authDialog.error} onClose={closeAuthDialog} />
{/if}
