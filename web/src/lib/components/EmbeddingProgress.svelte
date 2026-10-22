<script lang="ts">
  import { onMount } from 'svelte';
  import { Button, Card, SectionLabel } from '#lib/ui/index.js';
  import type { EmbedProgress } from '#lib/types.js';
  import {
    MEASURING,
    coveredPercent,
    formatClock,
    formatCount,
    formatEta,
    formatRate,
  } from '#lib/embedProgress.js';

  const REPO_URL = 'https://github.com/strelov1/freehire-volunteer-embed';
  const GUIDE_URL = `${REPO_URL}/blob/main/HOW_TO_RUN_THE_CLIENT.md`;

  // `initial` is the server-rendered document; the stream replaces it on every sample.
  let { initial, streamUrl }: { initial: EmbedProgress; streamUrl: string } = $props();

  let live = $state<EmbedProgress | null>(null);
  const progress = $derived(live ?? initial);
  const percent = $derived(coveredPercent(progress.covered, progress.eligible));

  onMount(() => {
    const source = new EventSource(streamUrl);
    source.addEventListener('progress', (e) => {
      try {
        live = JSON.parse((e as MessageEvent).data) as EmbedProgress;
      } catch {
        // a malformed frame is dropped; the next one is a whole document again
      }
    });
    // EventSource reconnects on its own; a dropped connection needs nothing here.
    return () => source.close();
  });
</script>

<Card class="p-6 sm:p-8">
  <div class="flex items-baseline justify-between gap-4">
    <p class="text-5xl font-semibold tracking-tighter tabular-nums">
      {percent === null ? MEASURING : `${percent}%`}
    </p>
    <p class="text-sm text-muted-foreground">of eligible postings embedded</p>
  </div>

  <div
    class="mt-6 h-3 w-full overflow-hidden rounded-full bg-muted"
    role="progressbar"
    aria-label="Postings embedded"
    aria-valuemin={0}
    aria-valuemax={100}
    aria-valuenow={percent ?? undefined}
  >
    <div class="h-full rounded-full bg-brand transition-all duration-700" style:width="{percent ?? 0}%"></div>
  </div>

  <dl class="mt-8 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-3">
    <div>
      <dt class="text-xs text-muted-foreground">Embedded</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatCount(progress.covered)}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted-foreground">Eligible</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatCount(progress.eligible)}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted-foreground">In the queue</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatCount(progress.queued)}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted-foreground">Speed</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatRate(progress.rate_per_minute)}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted-foreground">Time left</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatEta(progress.eta_seconds, progress.queued)}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted-foreground">Queue measured</dt>
      <dd class="mt-1 text-xl font-medium tabular-nums">{formatClock(progress.measured_at)}</dd>
    </div>
  </dl>

  <p class="mt-8 text-sm leading-relaxed text-muted-foreground">
    “Embedded” is eligible postings minus the queue, current to the queue’s last sample. A full
    count — {formatCount(progress.snapshot_covered)} embedded — was last taken at
    {formatClock(progress.snapshot_at)}; it runs every few hours because it reads every open
    posting.
  </p>
</Card>

<section class="mt-10">
  <SectionLabel text="help speed it up" />
  <h2 class="mt-3 text-2xl font-semibold tracking-tight">Lend your machine for a while.</h2>
  <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">
    The volunteer client runs the embedding model on your own computer and sends back vectors for
    postings in the queue. Pick how much of your machine it may use — easy, medium or hard — and
    watch the queue above move faster.
  </p>
  <div class="mt-5 flex flex-wrap gap-3">
    <Button href={GUIDE_URL} variant="primary">How to run the client</Button>
    <Button href={REPO_URL} variant="outline">Source on GitHub</Button>
  </div>
</section>
