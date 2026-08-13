<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Bell } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { notificationCenter } from '$lib/notificationCenter.svelte';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import States from './States.svelte';
  import NotificationCard from './NotificationCard.svelte';

  // The header's notification-center bell: unread badge + a dropdown of recorded
  // subscription-digest/reminder/nudge deliveries (see openspec/changes/
  // add-notification-center). Sits beside HeaderMenu in TopBar, same icon-button
  // treatment. Hidden entirely for signed-out visitors — there is nothing to show.
  // The full, paginated list lives at /my/notifications/history; this panel is
  // a recent-first glance, not the only way to reach older notifications.

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

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

  afterNavigate(() => {
    open = false;
  });

  function toggle(e: MouseEvent) {
    // Stop the toggle's own click from reaching the window outside-handler (see
    // HeaderMenu's identical guard).
    e.stopPropagation();
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
      <div
        class="absolute right-0 top-full z-50 mt-2 w-[calc(100vw-2rem)] max-w-96 overflow-y-auto rounded-md border border-border bg-background shadow-lg sm:w-96"
        style="max-height: min(70vh, 32rem);"
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
            {#each notificationCenter.items as item (item.id)}
              <li>
                <NotificationCard {item} onactivate={() => (open = false)} />
              </li>
            {/each}
          </ul>
        {/if}

        <div class="border-t border-border p-2">
          <a
            href={resolve('/my/notifications/history')}
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
