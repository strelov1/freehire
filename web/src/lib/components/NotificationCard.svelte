<script lang="ts">
  import { resolve } from '$app/paths';
  import { Search, Clock, Target, Archive, MessageCircle, Bell } from '@lucide/svelte';
  import { notificationCenter } from '$lib/notificationCenter.svelte';
  import { notificationTarget } from '$lib/notificationTarget';
  import { timeAgo } from '$lib/utils';
  import { cn } from '$lib/ui';
  import type { NotificationItem, NotificationKind } from '$lib/types';

  // One notification-center row, shared by the header bell's dropdown and the
  // full /my/notifications/history list so the two never drift. Renders as a
  // real `<a href>` when it has somewhere to go (ctrl/cmd-click, middle-click,
  // "open in new tab", and hover-preview all work as a browser expects), or a
  // plain button when it doesn't (a multi-job digest — nothing to open, but the
  // tap should still mark it read).

  let { item, onactivate }: { item: NotificationItem; onactivate?: () => void } = $props();

  const KIND_ICON: Record<NotificationKind, typeof Bell> = {
    subscription_digest: Search,
    reminder: Clock,
    nudge_follow_up: MessageCircle,
    nudge_interview_prep: Target,
    nudge_job_closed: Archive,
  };

  const target = $derived(notificationTarget(item));
  const href = $derived(
    target.kind === 'job'
      ? resolve('/jobs/[slug]', { slug: target.slug })
      : target.kind === 'tracking'
        ? resolve('/my/tracking')
        : null,
  );
  const unread = $derived(item.read_at == null);
  const Icon = $derived(KIND_ICON[item.kind]);

  const rowClass = $derived(
    cn(
      'flex w-full items-start gap-3 border-b border-border px-3 py-3 text-left transition-colors last:border-b-0 hover:bg-accent/50',
      unread && 'bg-brand/5',
    ),
  );

  // The click always marks read; navigation (if any) proceeds via the anchor's
  // own href — nothing here calls goto() or preventDefault()s the click.
  function activate() {
    void notificationCenter.markRead(item.id);
    onactivate?.();
  }
</script>

{#snippet body()}
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
{/snippet}

{#if href}
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- href is built from resolve() above; the rule can't see through the $derived indirection -->
  <a {href} onclick={activate} class={rowClass}>
    {@render body()}
  </a>
{:else}
  <button type="button" onclick={activate} class={rowClass}>
    {@render body()}
  </button>
{/if}
