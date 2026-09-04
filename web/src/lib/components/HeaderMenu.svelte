<script lang="ts">
  import { page } from '$app/state';
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import {
    Menu,
    X,
    Sun,
    Moon,
    CircleUser,
    Activity,
    ListChecks,
    BellRing,
    KeyRound,
    Inbox,
    Bot,
    ScrollText,
    FileText,
    SquarePlus,
    ShieldCheck,
    LogOut,
    LogIn,
  } from '@lucide/svelte';
  import { isAuthenticated, currentUser, logout as doLogout } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import { themeStore } from '$lib/theme.svelte';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { openedOverlay, closedOverlay } from '$lib/headerOverlay';
  import { cn } from '$lib/ui';
  import BrandMark from './BrandMark.svelte';
  import GithubStars from './GithubStars.svelte';
  import NotificationBell from './NotificationBell.svelte';
  import { ProviderIcon } from '$lib/ui';
  import { NAV } from '$lib/siteNav';

  // Same invite link as the footer's socials row (Footer.svelte) — no shared
  // constant exists for it yet, so it's kept inline here like GITHUB_URL's
  // counterpart in Footer.
  const DISCORD_URL = 'https://discord.gg/sYnZksswR';

  // The single menu absorbs the site nav, the signed-in account items, the theme
  // toggle, and the auth action — the header's only control besides search.
  //
  // Two layouts from one markup: on mobile the panel is a full-screen drawer
  // (own top bar · scrollable sectioned links · pinned bottom action bar); on
  // desktop it stays the small anchored dropdown. The theme toggle lives inside
  // the dropdown for both layouts — the bar itself only carries profile/sign-in.

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
  // Shared icon-button treatment for the bar controls (Discord, profile/sign-in,
  // menu).
  const iconButton =
    'size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  // The theme button only renders inside the open panel (client-only), so no
  // SSR mounted-guard is needed — read the store directly.
  const isDark = $derived(themeStore.isDark);
  const email = $derived(currentUser()?.email ?? '');
  const isModerator = $derived(currentUser()?.role === 'moderator');

  // Static nav (always shown) and the signed-in account items (shown only when
  // authenticated). Moderation is gated on the moderator role at render time.
  // Primary destinations pinned to the very top of the menu. Jobs is the feed at
  // /jobs — the homepage is the landing page above it, reachable via the logo.
  const primaryLinks = [NAV.jobs, NAV.companies];

  // About is rendered on its own at the foot of the list, so its glyph is lifted out
  // of NAV here rather than spelled a second time.
  const AboutIcon = NAV.about.icon;

  // The rest of the site. The two feature pages here are what the product DOES beyond
  // listing jobs, and until now nothing in this menu led to either of them — the
  // /features/* landings were reachable only from the pages that happened to link
  // them, which for a signed-out visitor was almost nowhere.
  //
  // A signed-in visitor also has "Tailor" and "Search alerts" in the account section
  // above. Those are the app; these are what it is for, and the divider between the
  // sections is what tells them apart — so the labels here name the subject
  // ("CV tailoring") rather than the tool.
  // How it works leads this group rather than sitting beside /about at the bottom:
  // /about is who publishes the site, this is how the thing in front of you works, and
  // it is the answer a first-time visitor is looking for. The homepage header names it
  // too, but only there and only above 640px — this is where it is always reachable.
  const navLinks = [
    NAV.collections,
    NAV.howItWorks,
    NAV.cvTailoring,
    NAV.jobNotifications,
    NAV.analytics,
    NAV.trends,
    NAV.discussions,
  ];

  // Personal account items — what the signed-in user owns/reads, in the same order
  // as the account sidebar (Profile is rendered separately just above these, so the
  // full run reads Profile · Activity · Tracking · Inbox · …). "Submit a job" is a
  // create action, rendered separately (below), split off from the "My submissions"
  // reading item it used to sit next to.
  const accountLinks = [
    { href: '/my/activity', label: 'Activity', icon: Activity },
    { href: '/my/tracking', label: 'Tracking', icon: ListChecks },
    { href: '/my/inbox', label: 'Inbox', icon: Inbox },
    // The agent and the tailoring list are reached from anywhere, not only from the account
    // shell, so they are duplicated here beside the inbox rather than left one level deeper.
    { href: '/my/assistant', label: 'Agent', icon: Bot },
    { href: '/my/cvs', label: 'Tailor', icon: ScrollText },
    { href: '/my/notifications/searches', label: 'Search alerts', icon: BellRing },
    { href: '/my/api-keys', label: 'API keys', icon: KeyRound },
    { href: '/my/submissions', label: 'My submissions', icon: FileText },
  ] as const;

  // Mobile only: the open panel is a full-screen overlay, so lock the page behind
  // it. Desktop keeps the small anchored dropdown and stays scrollable.
  $effect(() => {
    if (!open) return;
    if (window.matchMedia('(min-width: 640px)').matches) return;
    lockScroll();
    return () => unlockScroll();
  });

  function closeSelf() {
    open = false;
  }

  // Close whatever other header overlay (the bell dropdown, search suggestions)
  // was open, and let them close this one back — see headerOverlay.ts.
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
    open = false;
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

<!-- Theme toggle and auth action: defined once, reused across layouts. Both live
     inside the dropdown — theme in the mobile bottom bar and, on desktop, inline
     at the end of the link list alongside auth. -->
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
      <LogOut class="size-4 shrink-0" /> Log out
    </button>
  {:else}
    <button
      type="button"
      role="menuitem"
      onclick={signIn}
      class={cn(rowBase, 'w-full text-left font-medium text-foreground')}
    >
      <LogIn class="size-4 shrink-0" /> Sign in
    </button>
  {/if}
{/snippet}

<div class="relative flex items-center gap-1" bind:this={root}>
  <!-- Desktop bar order: GitHub stars, Discord, then the visitor's OWN two controls —
       the bell and profile/sign-in — then the menu button pinned to the far right. The
       bell used to sit ahead of the project's links, which put two icons about the site
       between it and the account it notifies about. On mobile the two about the site
       collapse into the drawer (below) and the two that are the visitor's stay: the
       bell is how they learn something happened, and it cannot be a tap deeper than
       the menu that would tell them. -->
  <GithubStars class="hidden sm:inline-flex" />

  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Discord invite, not an internal route -->
  <a
    href={DISCORD_URL}
    target="_blank"
    rel="noreferrer"
    aria-label="freehire on Discord"
    class={cn('hidden sm:inline-flex', iconButton)}
  >
    <ProviderIcon provider="discord" />
  </a>

  <NotificationBell />

  <!-- Desktop only: profile (signed in) or sign-in (signed out) sits before the
       menu button. -->
  {#if isAuthenticated()}
    <a
      href={resolve('/my/profile')}
      aria-label="Profile"
      title={email}
      class={cn('hidden sm:inline-flex', iconButton)}
    >
      <CircleUser class="size-5" />
    </a>
  {:else}
    <button
      type="button"
      aria-label="Sign in"
      onclick={signIn}
      class={cn('hidden sm:inline-flex', iconButton)}
    >
      <LogIn class="size-5" />
    </button>
  {/if}

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
          freehire
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
          {@const Icon = link.icon}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            <Icon class="size-4 shrink-0" />
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
            <CircleUser class="size-4 shrink-0" />
            Profile
          </a>
          {#each accountLinks as link (link.href)}
            {@const Icon = link.icon}
            <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
              <Icon class="size-4 shrink-0" />
              {link.label}
            </a>
          {/each}

          <!-- Create/action items, split off from the account reading items above. -->
          <div class="my-1 h-px bg-border"></div>
          <a href={resolve('/submit')} role="menuitem" onclick={() => (open = false)} class={linkClass('/submit')}>
            <SquarePlus class="size-4 shrink-0" />
            Submit a job
          </a>
          {#if isModerator}
            <a
              href={resolve('/moderation')}
              role="menuitem"
              onclick={() => (open = false)}
              class={linkClass('/moderation')}
            >
              <ShieldCheck class="size-4 shrink-0" />
              Moderation
            </a>
          {/if}
          <div class="my-1 h-px bg-border"></div>
        {/if}

        {#each navLinks as link (link.href)}
          {@const Icon = link.icon}
          <a href={resolve(link.href)} role="menuitem" onclick={() => (open = false)} class={linkClass(link.href)}>
            <Icon class="size-4 shrink-0" />
            {link.label}
          </a>
        {/each}

        <!-- About sits at the very bottom of the link list, just before the
             Sign in / Log out action (the marketing landing lives at /about). Read from
             NAV like every other destination — spelled here it would be a second copy
             of a page the header row already draws. -->
        <a
          href={resolve(NAV.about.href)}
          role="menuitem"
          onclick={() => (open = false)}
          class={linkClass(NAV.about.href)}
        >
          <AboutIcon class="size-4 shrink-0" />
          About
        </a>

        <!-- Desktop-only: theme toggle + auth inline at the end of the dropdown. -->
        <div class="hidden sm:block">
          <div class="my-1 h-px bg-border"></div>
          {@render themeButton()}
          {@render authButton()}
        </div>
      </div>

      <!-- Mobile-only: GitHub + theme + auth pinned to the bottom of the drawer. -->
      <div class="shrink-0 border-t border-border p-2 sm:hidden">
        <GithubStars variant="row" class={cn(rowBase, 'text-muted-foreground')} />
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Discord invite, not an internal route -->
        <a
          href={DISCORD_URL}
          target="_blank"
          rel="noreferrer"
          role="menuitem"
          aria-label="freehire on Discord"
          class={cn(rowBase, 'text-muted-foreground')}
        >
          <ProviderIcon provider="discord" class="size-4 shrink-0" />
          <span>Discord</span>
        </a>
        {@render themeButton()}
        {@render authButton()}
      </div>
    </div>
  {/if}
</div>
