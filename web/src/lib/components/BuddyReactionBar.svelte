<script lang="ts">
  import { ChevronUp, Clover, Flame, Handshake, Lightbulb, PartyPopper } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { REACTIONS, applyReaction, defaultReaction, reactionSummary } from '#lib/buddyReactions.js';
  import type { BuddyFeedItem, BuddyReactions, ReactionKind } from '#lib/types.js';
  import { cn } from '#lib/ui/index.js';

  // Reacting to a feed post. One click gives the reaction that fits the news (congrats for
  // an offer, luck for an interview); hovering — or the arrow, for a keyboard or a touch
  // screen — opens all five. Choosing your current one again takes it back. The screen
  // moves first and the server's answer settles it; a failure puts it back.
  let { eventId, type, reactions }: { eventId: number; type: BuddyFeedItem['type']; reactions: BuddyReactions } = $props();

  // Each kind's mark and colour, from the palette's tokens: the warmer the news, the warmer it reads.
  const LOOK: Record<ReactionKind, { icon: typeof PartyPopper; tone: string }> = {
    congrats: { icon: PartyPopper, tone: 'bg-brand-muted text-brand-strong' },
    luck: { icon: Clover, tone: 'bg-success-muted text-success-strong' },
    rooting: { icon: Flame, tone: 'bg-warning-muted text-warning-strong' },
    same: { icon: Handshake, tone: 'bg-secondary text-foreground' },
    insight: { icon: Lightbulb, tone: 'bg-accent text-foreground' },
  };
  const LABEL = Object.fromEntries(REACTIONS.map((r) => [r.kind, r.label])) as Record<ReactionKind, string>;

  // Follows the prop (a fresh read of the feed brings the server's numbers) and is written
  // locally while the viewer reacts.
  let current = $derived<BuddyReactions>(reactions);
  let open = $state(false);
  let busy = $state(false);

  const shown = $derived<ReactionKind>(current.mine || defaultReaction(type));
  const ShownIcon = $derived(LOOK[shown].icon);
  const summary = $derived(reactionSummary(current.counts));

  async function choose(kind: ReactionKind) {
    if (busy) return;
    open = false;
    const next = current.mine === kind ? '' : kind;
    const before = current;
    current = applyReaction(current, next);
    busy = true;
    try {
      current = next ? await api.reactToBuddyEvent(eventId, next) : await api.clearBuddyReaction(eventId);
    } catch {
      current = before;
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex items-center justify-between gap-2">
  <!-- The picker opens on hover and on focus within, so a pointer and a keyboard both reach it. -->
  <div class="group/react relative flex items-center" role="group" aria-label="React">
    <button
      type="button"
      aria-pressed={current.mine !== ''}
      disabled={busy}
      onclick={() => void choose(shown)}
      class={cn(
        'inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm font-medium transition-colors hover:bg-muted',
        current.mine ? 'text-brand-strong' : 'text-muted-foreground',
      )}
    >
      <ShownIcon class="size-4" aria-hidden="true" />{LABEL[shown]}
    </button>
    <button
      type="button"
      aria-label="More reactions"
      aria-expanded={open}
      onclick={() => (open = !open)}
      class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"
    >
      <ChevronUp class="size-4" aria-hidden="true" />
    </button>
    <div
      class={cn(
        'absolute bottom-full left-0 z-10 mb-1 flex gap-1 rounded-full border border-border bg-popover p-1.5 shadow-lg transition',
        open
          ? 'visible opacity-100'
          : 'invisible translate-y-1 opacity-0 group-focus-within/react:visible group-focus-within/react:translate-y-0 group-focus-within/react:opacity-100 group-hover/react:visible group-hover/react:translate-y-0 group-hover/react:opacity-100',
      )}
    >
      {#each REACTIONS as r (r.kind)}
        {@const Icon = LOOK[r.kind].icon}
        <button
          type="button"
          title={r.label}
          aria-label={r.label}
          aria-pressed={current.mine === r.kind}
          onclick={() => void choose(r.kind)}
          class={cn(
            'grid size-9 place-items-center rounded-full transition-transform hover:-translate-y-1 hover:scale-110 focus-visible:-translate-y-1',
            LOOK[r.kind].tone,
            current.mine === r.kind && 'ring-2 ring-brand',
          )}
        >
          <Icon class="size-4.5" aria-hidden="true" />
        </button>
      {/each}
    </div>
  </div>

  {#if summary.total > 0}
    <p class="flex items-center gap-1.5 text-xs text-muted-foreground tabular-nums">
      <span class="flex" aria-hidden="true">
        {#each summary.top as kind, i (kind)}
          {@const Icon = LOOK[kind].icon}
          <span class={cn('grid size-5 place-items-center rounded-full ring-2 ring-card', LOOK[kind].tone, i > 0 && '-ml-1.5')}>
            <Icon class="size-3" />
          </span>
        {/each}
      </span>
      <span>{summary.total}<span class="sr-only"> {summary.total === 1 ? 'reaction' : 'reactions'}</span></span>
    </p>
  {/if}
</div>
