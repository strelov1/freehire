<script lang="ts">
  import { ThumbsUp, ThumbsDown } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import type { VoteResult } from '$lib/types';

  // Shared thumbs up/down control for a job or a company. The aggregate counters
  // are public (shown to everyone); casting requires a session, so an anonymous
  // tap opens the sign-in dialog instead of calling the endpoint. The server is the
  // source of truth — we render its returned counters and my_vote after each write.
  let {
    target,
    slug,
    upvoteCount,
    downvoteCount,
    myVote = 0,
  }: {
    target: 'job' | 'company';
    slug: string;
    upvoteCount: number;
    downvoteCount: number;
    myVote?: number;
  } = $props();

  // Local reactive state seeded from props; updated optimistically-then-authoritatively.
  let up = $state(upvoteCount);
  let down = $state(downvoteCount);
  let mine = $state(myVote);
  let busy = $state(false);

  // Per-thumb pop animation flags (YouTube-style bounce). Set true on activation,
  // cleared on animationend; prefers-reduced-motion disables the keyframe via CSS.
  let popUp = $state(false);
  let popDown = $state(false);

  async function cast(dir: 'up' | 'down') {
    if (busy) return;
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
    }
    // Bounce the tapped thumb (unless it is the one being un-cast — a cleared vote
    // still gets a tap acknowledgement, so animate regardless of direction of change).
    if (dir === 'up') popUp = true;
    else popDown = true;

    busy = true;
    try {
      const wasMine = mine === (dir === 'up' ? 1 : -1);
      const res: VoteResult =
        target === 'job'
          ? wasMine
            ? await api.clearJobVote(slug)
            : await api.voteJob(slug, dir)
          : wasMine
            ? await api.clearCompanyVote(slug)
            : await api.voteCompany(slug, dir);
      up = res.upvote_count;
      down = res.downvote_count;
      mine = res.my_vote;
    } catch (e) {
      // A failed vote leaves the displayed counts untouched (they were never
      // optimistically changed). Re-prompt sign-in if the session lapsed.
      if (e instanceof ApiError && e.status === 401) openAuthDialog();
    } finally {
      busy = false;
    }
  }
</script>

<div class="vote" role="group" aria-label="Rate this {target}">
  <button
    type="button"
    class={['thumb', 'up', { active: mine === 1, popping: popUp }]}
    aria-pressed={mine === 1}
    aria-label="Thumbs up"
    disabled={busy}
    onclick={() => cast('up')}
    onanimationend={() => (popUp = false)}
  >
    <ThumbsUp size={18} />
    {#if up > 0}<span class="count">{up}</span>{/if}
  </button>

  <button
    type="button"
    class={['thumb', 'down', { active: mine === -1, popping: popDown }]}
    aria-pressed={mine === -1}
    aria-label="Thumbs down"
    disabled={busy}
    onclick={() => cast('down')}
    onanimationend={() => (popDown = false)}
  >
    <ThumbsDown size={18} />
    {#if down > 0}<span class="count">{down}</span>{/if}
  </button>
</div>

<style>
  .vote {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
  }

  .thumb {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.375rem 0.75rem;
    border: 1px solid var(--border, #d4d4d8);
    border-radius: 999px;
    background: var(--surface, #fff);
    color: var(--text-muted, #52525b);
    font-size: 0.875rem;
    font-variant-numeric: tabular-nums;
    cursor: pointer;
    transition:
      color 0.15s ease,
      border-color 0.15s ease,
      background 0.15s ease;
  }

  .thumb:hover:not(:disabled) {
    border-color: var(--border-strong, #a1a1aa);
  }

  .thumb:disabled {
    cursor: default;
    opacity: 0.7;
  }

  .thumb.up.active {
    color: var(--success, #16a34a);
    border-color: var(--success, #16a34a);
    background: color-mix(in srgb, var(--success, #16a34a) 10%, transparent);
  }

  .thumb.down.active {
    color: var(--danger, #dc2626);
    border-color: var(--danger, #dc2626);
    background: color-mix(in srgb, var(--danger, #dc2626) 10%, transparent);
  }

  /* YouTube-style tactile bounce on the tapped thumb's icon. */
  .thumb.popping :global(svg) {
    animation: thumb-pop 0.35s ease;
  }

  @keyframes thumb-pop {
    0% {
      transform: scale(1);
    }
    35% {
      transform: scale(1.35) rotate(-6deg);
    }
    70% {
      transform: scale(0.92);
    }
    100% {
      transform: scale(1);
    }
  }

  /* Respect reduced-motion: switch state instantly, no bounce. */
  @media (prefers-reduced-motion: reduce) {
    .thumb.popping :global(svg) {
      animation: none;
    }
  }
</style>
