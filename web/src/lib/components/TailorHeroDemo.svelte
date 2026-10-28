<!-- The tailoring loop in miniature for the landing hero: bullets rewritten one at a
     time toward a vacancy while the job match climbs, and one requirement left as a
     question rather than written in. Illustrative — the copy is DEMO_* in tailorDemo.ts.
     Rendered on the server, and under reduced motion, as its finished state. -->
<script lang="ts">
  import {
    DEMO_ASKED,
    DEMO_BULLETS,
    DEMO_REQUIREMENTS,
    DEMO_ROLE,
    DEMO_SCORES,
  } from '#lib/tailorDemo.js';

  const STEP_MS = 1800;
  const HOLD_TICKS = 2;
  const LAST = DEMO_BULLETS.length;
  const ASKED_LABEL = DEMO_REQUIREMENTS.find((r) => r.id === DEMO_ASKED)?.label;

  let step = $state(LAST);
  let root: HTMLElement | undefined = $state();

  const score = $derived(DEMO_SCORES[step]);
  const met = $derived(new Set(DEMO_BULLETS.slice(0, step).map((b) => b.req)));

  function chipClass(id: string): string {
    if (met.has(id)) return 'border-success-strong/50 text-success-strong';
    if (id === DEMO_ASKED && step === LAST) return 'border-warning/50 text-warning-strong';
    return 'border-border text-muted-foreground';
  }

  $effect(() => {
    if (!root || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    let tick = 0;
    const start = () => {
      tick = 0;
      step = 0;
      timer = setInterval(() => {
        tick = (tick + 1) % (LAST + 1 + HOLD_TICKS);
        step = Math.min(tick, LAST);
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
    <span>freehire · Tailoring</span>
    <span>example</span>
  </figcaption>
  <p class="sr-only">
    An example: the agent rewrites {LAST} CV bullets toward a backend payments role, the job match
    rises from {DEMO_SCORES[0]} to {DEMO_SCORES[LAST]}, and it asks about {ASKED_LABEL}
    instead of writing them in.
  </p>
  <div aria-hidden="true" class="grid gap-px bg-border sm:grid-cols-5">
    <div class="bg-background p-4 sm:col-span-2">
      <p class="text-xs uppercase tracking-wide text-muted-foreground">Vacancy</p>
      <p class="mt-1 text-sm font-semibold">{DEMO_ROLE}</p>
      <ul class="mt-3 flex flex-wrap gap-1.5">
        {#each DEMO_REQUIREMENTS as r (r.id)}
          <li class="rounded-md border px-2 py-0.5 font-mono text-xs transition-colors duration-500 {chipClass(r.id)}">
            {r.label}
          </li>
        {/each}
      </ul>
      <p
        class="mt-3 text-xs text-warning-strong transition-opacity duration-500 {step === LAST
          ? 'opacity-100'
          : 'invisible opacity-0'}"
      >
        {ASKED_LABEL}: not in your history — asked you first.
      </p>
    </div>
    <div class="relative bg-background p-4 sm:col-span-3">
      <div class="absolute right-4 top-4 text-right">
        <p class="text-3xl font-semibold tabular-nums">{score}</p>
        <p class="text-xs text-muted-foreground">job match</p>
      </div>
      <p class="text-xs uppercase tracking-wide text-muted-foreground">Your CV</p>
      <ul class="mt-3 space-y-2 pr-20 text-xs leading-snug">
        {#each DEMO_BULLETS as b, i (b.req)}
          <li class="grid">
            <span
              class="col-start-1 row-start-1 text-muted-foreground transition-opacity duration-500 {i < step
                ? 'opacity-0'
                : 'opacity-100'}"
            >
              {b.before}
            </span>
            <span
              class="col-start-1 row-start-1 self-start rounded bg-brand/10 text-foreground transition-opacity duration-500 {i < step
                ? 'opacity-100'
                : 'opacity-0'}"
            >
              {b.after}
            </span>
          </li>
        {/each}
      </ul>
    </div>
  </div>
</figure>
