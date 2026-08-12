<script lang="ts">
  import { goto, afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Bell, Search, Clock, Target, Archive, MessageCircle } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { notificationCenter } from '$lib/notificationCenter.svelte';
  import { notificationTarget } from '$lib/notificationTarget';
  import { timeAgo } from '$lib/utils';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';
  import { cn } from '$lib/ui';
  import States from './States.svelte';
  import type { NotificationItem, NotificationKind } from '$lib/types';

  // The header's notification-center bell: unread badge + a dropdown of recorded
  // subscription-digest/reminder/nudge deliveries (see openspec/changes/
  // add-notification-center). Sits beside HeaderMenu in TopBar, same icon-button
  // treatment. Hidden entirely for signed-out visitors — there is nothing to show.

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const KIND_ICON: Record<NotificationKind, typeof Bell> = {
    subscription_digest: Search,
    reminder: Clock,
    nudge_follow_up: MessageCircle,
    nudge_interview_prep: Target,
    nudge_job_closed: Archive,
  };

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

  async function tap(item: NotificationItem) {
    open = false;
    const target = notificationTarget(item);
    void notificationCenter.markRead(item.id);
    if (target.kind === 'job') void goto(resolve('/jobs/[slug]', { slug: target.slug }));
    else if (target.kind === 'tracking') void goto(resolve('/my/tracking'));
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
      aria-haspopup="menu"
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
        role="menu"
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
              {@const Icon = KIND_ICON[item.kind]}
              {@const unread = item.read_at == null}
              <li>
                <button
                  type="button"
                  role="menuitem"
                  onclick={() => tap(item)}
                  class={cn(
                    'flex w-full items-start gap-3 border-b border-border px-3 py-3 text-left transition-colors last:border-b-0 hover:bg-accent/50',
                    unread && 'bg-brand/5',
                  )}
                >
                  <Icon class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                  <span class="min-w-0 flex-1">
                    <span class={cn('block text-sm', unread ? 'font-semibold text-foreground' : 'font-medium text-foreground')}>
                      {item.title}
                    </span>
                    <span class="mt-0.5 block text-xs text-muted-foreground">{item.body}</span>
                    <span class="mt-1 block text-[11px] text-muted-foreground">{timeAgo(item.created_at)}</span>
                  </span>
                  {#if unread}
                    <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-brand" aria-label="unread"></span>
                  {/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  </div>
{/if}
