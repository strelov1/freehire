<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { CircleUser, SquarePlus, ShieldCheck, LogOut, LogIn } from '@lucide/svelte';
  import { isAuthenticated, currentUser, logout as doLogout } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import { openedOverlay, closedOverlay } from '#lib/headerOverlay.js';
  import { cn } from '#lib/ui/index.js';
  import { menuNav } from '#lib/actions/menuNav.js';
  import { visibleAccountNav } from '#lib/accountNav.js';
  import { accountNavIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages as shellMessages, navLabel } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';

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

  // The account items are the sidebar's own list — same sections, order, icons and labels
  // — read from accountNav rather than a curated copy, which is how the two drifted apart.
  const shell = $derived(t(shellMessages, locale()));
  const accountItems = $derived(visibleAccountNav(isModerator, currentUser()?.beta_tester ?? false));
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

  afterNavigate(({ shallow }) => {
    // SvelteKit 3 fires this for shallow routing too; a URL write is not a page change.
    if (!shallow) open = false;
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

<!-- Signed in, the profile trigger lives here on wide screens only (the consolidated menu
     carries it on a phone). Signed out, Sign in shows at every width: a first-time visitor
     on a phone otherwise had no way in short of opening the menu. -->
<div class={['relative', isAuthenticated() ? 'hidden sm:inline-flex' : 'inline-flex']} bind:this={root}>
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
        aria-label="Profile"
        {@attach menuNav({ onclose: closeSelf })}
        class="profile-panel absolute right-0 top-full z-50 mt-2 flex w-56 flex-col rounded-md border border-border bg-background py-1 shadow-lg"
      >
        <!-- Only the sections scroll: the actions below stay in view, so Log out never
             needs a scroll to reach — the reason this menu was split off at all. -->
        <div class="min-h-0 flex-1 overflow-y-auto">
          {#each accountItems as item (item.href)}
            {@const Icon = accountNavIcons[item.href]}
            <a
              href={resolve(toPath(item.href))}
              role="menuitem"
              onclick={() => (open = false)}
              class={rowBase}
              title={item.href === '/my/profile' ? email : undefined}
            >
              <Icon class="size-4 shrink-0" />
              {navLabel(shell, item.href, item.label)}
            </a>
          {/each}
        </div>

        <div class="my-1 h-px shrink-0 bg-border"></div>
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
    <button
      type="button"
      onclick={signIn}
      class="inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-md border border-border px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <LogIn class="size-4" aria-hidden="true" /> Sign in
    </button>
  {/if}
</div>

<style>
  /* The panel lists every account section, which can be taller than a short window;
     it stays inside the viewport, and its section list scrolls instead. */
  .profile-panel {
    max-height: calc(100dvh - 5rem);
  }
</style>
