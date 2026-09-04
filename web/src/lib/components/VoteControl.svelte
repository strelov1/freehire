<script lang="ts">
  import { ThumbsUp, ThumbsDown } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import type { VoteResult } from '$lib/types';

  // Shared thumbs up/down control for a job or a company. The aggregate counters
  // are public (shown to everyone); casting requires a session, so an anonymous
  // tap sends the visitor to sign in instead of calling the endpoint. The server is
  // the source of truth — we render its returned counters and my_vote after each write.
  //
  // Styling is driven entirely by the design-system tokens (bg-card / border-border /
  // text-muted-foreground / accent / destructive), so the control adapts to the dark
  // and light themes automatically.
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
      promptSignIn();
      return;
    }
    if (dir === 'up') popUp = true;
    else popDown = true;

    busy = true;
    try {
      // Re-casting the direction the user already holds clears the vote; any other
      // tap casts it. Pick the right endpoint pair for the target, then act.
      const endpoints =
        target === 'job'
          ? { cast: api.voteJob, clear: api.clearJobVote }
          : { cast: api.voteCompany, clear: api.clearCompanyVote };
      const wasMine = mine === (dir === 'up' ? 1 : -1);
      const res: VoteResult = wasMine
        ? await endpoints.clear(slug)
        : await endpoints.cast(slug, dir);
      up = res.upvote_count;
      down = res.downvote_count;
      mine = res.my_vote;
    } catch (e) {
      // A failed vote leaves the displayed counts untouched (they were never
      // optimistically changed). Re-prompt sign-in if the session lapsed.
      if (e instanceof ApiError && e.status === 401) promptSignIn();
    } finally {
      busy = false;
    }
  }

  const base =
    'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-sm ' +
    'tabular-nums transition-colors disabled:cursor-default disabled:opacity-70';
</script>

<div class="inline-flex items-center gap-2" role="group" aria-label={`Rate this ${target}`}>
  <button
    type="button"
    class={[
      base,
      'thumb',
      mine === 1
        ? 'border-green-500/40 bg-green-500/10 text-green-600 dark:text-green-500'
        : 'border-border bg-card text-muted-foreground hover:bg-accent hover:text-foreground',
      { popping: popUp },
    ]}
    aria-pressed={mine === 1}
    aria-label={up > 0 ? `Thumbs up, ${up}` : 'Thumbs up'}
    disabled={busy}
    onclick={() => cast('up')}
    onanimationend={() => (popUp = false)}
  >
    <ThumbsUp size={18} />
    {#if up > 0}<span>{up}</span>{/if}
  </button>

  <button
    type="button"
    class={[
      base,
      'thumb',
      mine === -1
        ? 'border-destructive/40 bg-destructive/10 text-destructive'
        : 'border-border bg-card text-muted-foreground hover:bg-accent hover:text-foreground',
      { popping: popDown },
    ]}
    aria-pressed={mine === -1}
    aria-label={down > 0 ? `Thumbs down, ${down}` : 'Thumbs down'}
    disabled={busy}
    onclick={() => cast('down')}
    onanimationend={() => (popDown = false)}
  >
    <ThumbsDown size={18} />
    {#if down > 0}<span>{down}</span>{/if}
  </button>
</div>

<style>
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
