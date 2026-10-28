<!-- The interview-buddies loop in miniature for the landing hero: other members' progress
     arriving in the feed, then an invitation to the one who just had the interview.
     Illustrative — the copy is DEMO_* in buddiesDemo.ts. Rendered on the server, and under
     reduced motion, as its finished state. Every slot is always laid out and only its
     opacity changes, so the card never changes height mid-loop. -->
<script lang="ts">
  import { DEMO_FEED, DEMO_INVITE } from '#lib/buddiesDemo.js';

  const STEP_MS = 1800;
  const HOLD_TICKS = 2;
  const FEED_LEN = DEMO_FEED.length;
  const LAST = FEED_LEN + 1;

  const EVENT_TEXT = {
    applied: 'applied to',
    interview: 'reached the interview at',
    offer: 'got an offer from',
  } as const;

  let step = $state(LAST);
  let root: HTMLElement | undefined = $state();

  $effect(() => {
    if (!root || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    let tick = 0;
    const start = () => {
      tick = 0;
      step = 1;
      timer = setInterval(() => {
        tick = (tick + 1) % (LAST + HOLD_TICKS);
        step = Math.min(tick + 1, LAST);
      }, STEP_MS);
    };
    const stop = () => {
      clearInterval(timer);
      timer = undefined;
      step = LAST;
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

<figure bind:this={root} class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
  <figcaption
    class="flex items-center justify-between border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
  >
    <span>freehire · Interview buddies</span>
    <span>example</span>
  </figcaption>
  <p class="sr-only">
    An example feed: {FEED_LEN} anonymous members applying to and interviewing at the same companies
    as you, then a message to {DEMO_INVITE.to} asking how their interview went.
  </p>
  <div aria-hidden="true" class="bg-background p-4">
    <p class="text-xs uppercase tracking-wide text-muted-foreground">Feed · similar to you</p>
    <ul class="mt-3 divide-y divide-border">
      {#each DEMO_FEED as e, i (e.handle)}
        <li class="py-2.5 transition-opacity duration-500 {i < step ? 'opacity-100' : 'opacity-0'}">
          <div class="flex items-baseline justify-between gap-3">
            <span class="font-mono text-xs font-medium">{e.handle}</span>
            <span class="text-xs text-muted-foreground">{e.day}</span>
          </div>
          <p class="mt-0.5 text-sm">
            <span class="text-muted-foreground">{e.role} ·</span>
            {EVENT_TEXT[e.type]}
            <span class="font-medium {e.type === 'applied' ? '' : 'text-success-strong'}">{e.company}</span>
          </p>
        </li>
      {/each}
    </ul>
    <div
      class="ml-auto mt-3 max-w-xs rounded-lg bg-secondary px-3 py-2 transition-opacity duration-500 {step === LAST
        ? 'opacity-100'
        : 'invisible opacity-0'}"
    >
      <p class="text-xs text-muted-foreground">To {DEMO_INVITE.to}</p>
      <p class="mt-0.5 text-sm">{DEMO_INVITE.body}</p>
      <p class="mt-1 text-right text-xs text-muted-foreground">sent</p>
    </div>
  </div>
</figure>
