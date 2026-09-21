<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { CircleUser, SquarePlus, ShieldCheck, LogOut, LogIn } from '@lucide/svelte';
  import { isAuthenticated, currentUser, logout as doLogout } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import { openedOverlay, closedOverlay } from '$lib/headerOverlay';
  import { cn } from '$lib/ui';
  import { accountLinks } from '$lib/headerAccountLinks';

  // Desktop-only: the profile icon doubles as a second, independent dropdown
  // trigger scoped to the signed-in user's own account, split off HeaderMenu's
  // consolidated menu so Log out is reachable without scrolling past every site
  // nav link — see openspec/changes/split-header-profile-menu/design.md. Signed
  // out, it stays the existing direct Sign-in icon action: there is nothing to
  // list, so no dropdown.

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const email = $derived(currentUser()?.email ?? '');
  const isModerator = $derived(currentUser()?.role === 'moderator');
  // Drives the trigger's tier badge — see header-navigation's "Paying-tier badge on
  // the desktop profile icon" requirement. 'free' shows nothing.
  const tier = $derived(currentUser()?.tier ?? 'free');

  // Same row/icon-button treatment HeaderMenu's desktop dropdown uses, so the two
  // menus read as one visual language.
  const rowBase =
    'flex items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground';
  const iconButton =
    'size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  function closeSelf() {
    open = false;
  }

  // Close whatever other header overlay (the consolidated menu, the notification
  // bell) was open, and let them close this one back — see headerOverlay.ts.
  $effect(() => {
    if (!open) return;
    openedOverlay(closeSelf);
    return () => closedOverlay(closeSelf);
  });

  afterNavigate(() => {
    open = false;
  });

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  function signIn() {
    promptSignIn();
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

<div class="relative hidden sm:inline-flex" bind:this={root}>
  {#if isAuthenticated()}
    <button
      type="button"
      aria-label={tier === 'free' ? 'Profile' : `Profile (${tier})`}
      title={email}
      aria-haspopup="menu"
      aria-expanded={open}
      onclick={(e) => {
        // See HeaderMenu.svelte's own toggle for why this is needed: without it,
        // opening detaches the clicked element and onWindowClick immediately
        // re-closes the just-opened menu.
        e.stopPropagation();
        open = !open;
      }}
      class={cn('relative inline-flex', iconButton)}
    >
      <CircleUser class="size-5" />
      {#if tier !== 'free'}
        <!-- Same height/weight as NotificationBell's unread-count badge (h-4, bold, ring
             against the background) so the two corner badges read with equal prominence —
             the 8px/py-px version this replaced was easy to miss next to that one. -->
        <span
          aria-hidden="true"
          class="absolute -bottom-1 -right-1 flex h-4 items-center justify-center rounded-full bg-brand px-1.5 text-[10px] font-bold uppercase leading-none text-brand-foreground ring-2 ring-background"
        >
          {tier}
        </span>
      {/if}
    </button>

    {#if open}
      <div
        role="menu"
        class="absolute right-0 top-full z-50 mt-2 w-56 rounded-md border border-border bg-background py-1 shadow-lg"
      >
        <a href={resolve('/my/profile')} role="menuitem" onclick={() => (open = false)} class={rowBase} title={email}>
          <CircleUser class="size-4 shrink-0" />
          Profile
        </a>
        {#each accountLinks as link (link.href)}
          {@const Icon = link.icon}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={rowBase}>
            <Icon class="size-4 shrink-0" />
            {link.label}
          </a>
        {/each}

        <div class="my-1 h-px bg-border"></div>
        <a href={resolve('/submit')} role="menuitem" onclick={() => (open = false)} class={rowBase}>
          <SquarePlus class="size-4 shrink-0" />
          Submit a job
        </a>
        {#if isModerator}
          <a href={resolve('/moderation')} role="menuitem" onclick={() => (open = false)} class={rowBase}>
            <ShieldCheck class="size-4 shrink-0" />
            Moderation
          </a>
        {/if}

        <div class="my-1 h-px bg-border"></div>
        <button type="button" role="menuitem" onclick={logout} class={cn(rowBase, 'w-full text-left')}>
          <LogOut class="size-4 shrink-0" />
          Log out
        </button>
      </div>
    {/if}
  {:else}
    <button type="button" aria-label="Sign in" onclick={signIn} class={cn('inline-flex', iconButton)}>
      <LogIn class="size-5" />
    </button>
  {/if}
</div>
