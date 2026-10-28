<!-- The interview-buddies loop in miniature for the landing hero: other members' progress
     arriving in the feed, then an invitation to the one who just had the interview and
     their reply. Illustrative — the copy is DEMO_* in buddiesDemo.ts. Rendered on the
     server, and under reduced motion, as its finished state. Every slot is always laid out
     and only its opacity/offset changes, so the card never changes height mid-loop. -->
<script lang="ts">
  import { BadgeCheck, MessagesSquare, Send, User } from '@lucide/svelte';
  import { DEMO_FEED, DEMO_INVITE, DEMO_REPLY } from '#lib/buddiesDemo.js';
  import { buddyFeedEventLabel } from '#lib/buddies.js';
  import { Avatar, EntityLogo } from '#lib/ui/index.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  const STEP_MS = 1800;
  const HOLD_TICKS = 2;
  const FEED_LEN = DEMO_FEED.length;
  // step n (1..FEED_LEN) shows n entries; then typing, invite, reply.
  const TYPING = FEED_LEN + 1;
  const INVITE = FEED_LEN + 2;
  const REPLY = FEED_LEN + 3;

  const CUES = {
    applied: { icon: Send, tile: 'bg-muted text-muted-foreground' },
    interview: { icon: MessagesSquare, tile: 'bg-secondary text-secondary-foreground' },
    offer: { icon: BadgeCheck, tile: 'bg-brand-muted text-brand-strong' },
  } as const;

  let step = $state(REPLY);
  let root: HTMLElement | undefined = $state();

  $effect(() => {
    if (!root || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    let tick = 0;
    const start = () => {
      // Keep the finished state for the hold ticks, then restart from one entry.
      tick = REPLY;
      step = REPLY;
      timer = setInterval(() => {
        tick = (tick + 1) % (REPLY + HOLD_TICKS);
        step = Math.min(tick + 1, REPLY);
      }, STEP_MS);
    };
    const stop = () => {
      clearInterval(timer);
      timer = undefined;
      step = REPLY;
    };
    const io = new IntersectionObserver((entries) => {
      const visible = entries.at(-1)?.isIntersecting;
      if (visible && !timer) start();
      else if (visible === false && timer) stop();
    });
    io.observe(root);
    return () => {
      io.disconnect();
      clearInterval(timer);
    };
  });
</script>

{#snippet personIcon()}
  <User class="size-4" aria-hidden="true" />
{/snippet}

<figure bind:this={root} class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
  <figcaption
    class="flex items-center justify-between border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
  >
    <span>freehire · Interview buddies</span>
    <span class="flex items-center gap-2">
      <span class="relative flex size-2">
        <span class="absolute inline-flex size-full motion-safe:animate-ping rounded-full bg-brand opacity-75"></span>
        <span class="relative inline-flex size-2 rounded-full bg-brand"></span>
      </span>
      live · example
    </span>
  </figcaption>
  <p class="sr-only">
    An example feed: {FEED_LEN} anonymous members applying to and interviewing at the same companies
    as you, then a message to {DEMO_INVITE.to} asking how their interview went, and their reply.
  </p>
  <div aria-hidden="true" class="bg-background p-4">
    <p class="text-xs uppercase tracking-wide text-muted-foreground">Feed · similar to you</p>
    <ul class="mt-3 divide-y divide-border">
      {#each DEMO_FEED as e, i (e.handle)}
        {@const cue = CUES[e.type]}
        {@const Icon = cue.icon}
        <li
          class="flex flex-col gap-2 py-3 transition duration-500 {i < step
            ? 'translate-y-0 opacity-100'
            : 'translate-y-2 opacity-0'}"
        >
          <div class="flex items-start justify-between gap-3">
            <BuddyIdentity handle={e.handle} summary={e.summary} size="sm" link={false} />
            <span class="shrink-0 text-xs text-muted-foreground">{e.day}</span>
          </div>
          <div class="flex items-center gap-3">
            <span class="flex size-8 shrink-0 items-center justify-center rounded-md {cue.tile}">
              <Icon class="size-4" aria-hidden="true" />
            </span>
            <p class="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm">
              <span class={e.type === 'offer' ? 'font-medium text-brand-strong' : 'text-muted-foreground'}
                >{buddyFeedEventLabel(e.type)}</span
              >
              <span class="flex items-center gap-1.5">
                <EntityLogo name={e.company} shape="square" size="xs" />
                <span class="font-medium text-foreground">{e.company}</span>
              </span>
            </p>
          </div>
        </li>
      {/each}
    </ul>
    <div class="mt-3 flex flex-col gap-2">
      <div
        data-testid="typing"
        class="flex h-6 items-center gap-1 transition-opacity duration-300 {step === TYPING
          ? 'opacity-100'
          : 'invisible opacity-0'}"
      >
        <span class="size-1.5 motion-safe:animate-bounce rounded-full bg-muted-foreground"></span>
        <span class="size-1.5 motion-safe:animate-bounce rounded-full bg-muted-foreground delay-150"></span>
        <span class="size-1.5 motion-safe:animate-bounce rounded-full bg-muted-foreground delay-300"></span>
        <span class="ml-1 text-xs text-muted-foreground">typing…</span>
      </div>
      <div
        data-testid="invite"
        class="ml-auto max-w-xs rounded-lg bg-secondary px-3 py-2 transition duration-500 {step >= INVITE
          ? 'translate-y-0 opacity-100'
          : 'invisible translate-y-2 opacity-0'}"
      >
        <p class="text-xs text-muted-foreground">To {DEMO_INVITE.to}</p>
        <p class="mt-0.5 text-sm">{DEMO_INVITE.body}</p>
        <p class="mt-1 text-right text-xs text-muted-foreground">sent</p>
      </div>
      <div
        data-testid="reply"
        class="flex max-w-sm items-end gap-2 transition duration-500 {step >= REPLY
          ? 'translate-y-0 opacity-100'
          : 'invisible translate-y-2 opacity-0'}"
      >
        <Avatar size="sm" class="shrink-0 bg-secondary" fallbackIcon={personIcon} />
        <p class="rounded-lg bg-muted px-3 py-2 text-sm">{DEMO_REPLY.body}</p>
      </div>
    </div>
  </div>
</figure>
