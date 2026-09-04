<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Bell } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { notificationCenter } from '$lib/notificationCenter.svelte';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { openedOverlay, closedOverlay } from '$lib/headerOverlay';
  import States from './States.svelte';
  import NotificationCard from './NotificationCard.svelte';

  // The header's notification-center bell: unread badge + a dropdown of recorded
  // subscription-digest/reminder/nudge deliveries (see openspec/changes/
  // add-notification-center). Sits beside HeaderMenu in TopBar, same icon-button
  // treatment. Hidden entirely for signed-out visitors — there is nothing to show.
  // The full, paginated list lives at /my/notifications (the notification
  // center's History tab); this panel is a recent-first glance, not the only
  // way to reach older notifications — capped to 5 rows here so it stays a
  // glance, with "View all" below for the rest.
  const DROPDOWN_LIMIT = 5;

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  const visibleItems = $derived(notificationCenter.items.slice(0, DROPDOWN_LIMIT));

  // Load once the session is confirmed (boot-time /me may still be in flight); drop
  // the cached page on sign-out so the next user on this tab loads their own.
  $effect(() => {
    if (isAuthenticated()) {
      void notificationCenter.ensureLoaded();
    } else {
      notificationCenter.reset();
    }
  });

  // Lock body scroll only while the dropdown covers the screen on mobile.
  $effect(() => {
    if (!open) return;
    if (window.matchMedia('(min-width: 640px)').matches) return;
    lockScroll();
    return () => unlockScroll();
  });

  function closeSelf() {
    open = false;
  }

  // Close whatever other header overlay (search suggestions, the hamburger menu)
  // was open, and let them close this one back — see headerOverlay.ts for why a
  // click-outside check on each component's own root can't coordinate this: this
  // button sits inside HeaderMenu's own root, so a click on it is never "outside"
  // the menu.
  $effect(() => {
    if (!open) return;
    openedOverlay(closeSelf);
    return () => closedOverlay(closeSelf);
  });

  afterNavigate(() => {
    open = false;
  });

  function toggle() {
    open = !open;
    if (open) void notificationCenter.refresh();
  }

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  async function markAllRead(e: MouseEvent) {
    e.stopPropagation();
    await notificationCenter.markAllRead();
  }
</script>

<svelte:window
  onclick={onWindowClick}
  onkeydown={(e) => e.key === 'Escape' && (open = false)}
/>

{#if isAuthenticated()}
  <div class="relative" bind:this={root}>
    <button
      type="button"
      aria-label="Notifications"
      aria-haspopup="true"
      aria-expanded={open}
      onclick={toggle}
      class="relative inline-flex size-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Bell class="size-5" />
      {#if notificationCenter.unreadCount > 0}
        <span
          class="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-brand px-1 text-[10px] font-semibold leading-none text-brand-foreground"
        >
          {notificationCenter.unreadCount > 99 ? '99+' : notificationCenter.unreadCount}
        </span>
      {/if}
    </button>

    {#if open}
      <!-- On a phone the panel leaves the button and takes the screen: full width, from
           under the sticky header (`top-14` is that header's own `h-14`) down to
           `bottom-0` — same treatment as HeaderSearch's mobile suggestion list, and for
           the same reason: `fixed` rather than `absolute` because the button sits well
           right of the viewport edge, so no side margin reaches it. Works only because
           TopBar's header draws no `backdrop-filter` (see its own comment) — that would
           make the header the containing block and pin this panel to it instead. -->
      <div
        class="z-50 overflow-y-auto border border-border bg-background shadow-lg max-sm:fixed max-sm:inset-x-0 max-sm:bottom-0 max-sm:top-14 max-sm:border-x-0 max-sm:border-b-0 sm:absolute sm:right-0 sm:top-full sm:mt-2 sm:w-96 sm:max-h-[min(70vh,32rem)] sm:rounded-md"
      >
        <div class="flex items-center justify-between border-b border-border px-3 py-2">
          <span class="text-sm font-semibold">Notifications</span>
          {#if notificationCenter.unreadCount > 0}
            <button
              type="button"
              onclick={markAllRead}
              class="text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
            >
              Mark all read
            </button>
          {/if}
        </div>

        {#if !notificationCenter.loaded}
          <div class="p-3">
            <States state="loading" rows={3} />
          </div>
        {:else if notificationCenter.items.length === 0}
          <div class="p-3">
            <States state="empty" message="No notifications yet." />
          </div>
        {:else}
          <ul>
            {#each visibleItems as item (item.id)}
              <li>
                <NotificationCard {item} onactivate={() => (open = false)} />
              </li>
            {/each}
          </ul>
        {/if}

        <div class="border-t border-border p-2">
          <a
            href={resolve('/my/notifications')}
            onclick={() => (open = false)}
            class="block rounded px-2 py-1.5 text-center text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            View all notifications
          </a>
        </div>
      </div>
    {/if}
  </div>
{/if}
