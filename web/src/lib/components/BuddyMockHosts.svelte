<script lang="ts">
  import { Star, UsersRound } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { MOCK_TOPICS, hostRatingLine, hostSlotLabel, mockTopicLabel } from '#lib/buddyMocks.js';
  import type { BuddyMockHost, BuddyMockTopic } from '#lib/types.js';
  import { Button, Chip, EmptyState, Skeleton, tabsListVariants, tabsTriggerVariants } from '#lib/ui/index.js';
  import BuddyIdentity from './BuddyIdentity.svelte';
  import States from './States.svelte';

  // Who runs mocks: every member the viewer could book now, soonest free hour first, in the
  // viewer's own zone. Booking itself happens on the member's page, on its mock tab.
  // `hostingAnchor` is where the empty state sends someone to become the first host;
  // `hosting` says they already are one, which changes what an empty listing means.
  let { hostingAnchor, hosting }: { hostingAnchor: string; hosting: boolean } = $props();

  const zone = browserTimezone();
  let topic = $state<BuddyMockTopic | ''>('');
  let status = $state<'loading' | 'ready' | 'error'>('loading');
  let hosts = $state<BuddyMockHost[]>([]);
  let shownZone = $state(zone);
  let attempt = $state(0);

  const filters: readonly { value: BuddyMockTopic | ''; label: string }[] = [
    { value: '', label: 'All' },
    ...MOCK_TOPICS.map((t) => ({ value: t, label: mockTopicLabel(t) })),
  ];

  $effect(() => {
    const current = topic;
    void attempt;
    let cancelled = false;
    status = 'loading';
    api.listBuddyMockHosts(zone, current || undefined).then(
      (res) => {
        if (cancelled) return;
        hosts = res.hosts;
        shownZone = res.timezone;
        status = 'ready';
      },
      () => {
        if (!cancelled) status = 'error';
      },
    );
    return () => {
      cancelled = true;
    };
  });

  const bookHref = (handle: string) => `${resolve('/buddies/[handle]', { handle })}?tab=mock`;
</script>

<section class="flex flex-col gap-4 rounded-xl border border-border bg-card p-5" aria-labelledby="mock-hosts-heading">
  <div class="flex flex-col gap-3">
    <div class="flex flex-col gap-0.5">
      <h2 id="mock-hosts-heading" class="text-base font-semibold">Who runs mocks</h2>
      <p class="text-sm text-muted-foreground">
        Members with a free hour in the next 30 days, soonest first. Times in {shownZone}.
      </p>
    </div>
    <div class={tabsListVariants({ class: 'max-w-full justify-start overflow-x-auto' })} role="group" aria-label="Topic">
      {#each filters as f (f.value)}
        <button
          type="button"
          class={tabsTriggerVariants({ active: topic === f.value })}
          aria-pressed={topic === f.value}
          onclick={() => (topic = f.value)}
        >
          {f.label}
        </button>
      {/each}
    </div>
  </div>

  {#if status === 'loading'}
    <States state="loading" rows={3}>
      {#snippet row()}
        <div class="flex items-center gap-3 py-2">
          <Skeleton class="size-10 shrink-0 rounded-full" />
          <div class="flex flex-1 flex-col gap-1.5">
            <Skeleton class="h-4 w-40" />
            <Skeleton class="h-3 w-24" />
          </div>
          <Skeleton class="h-8 w-20" />
        </div>
      {/snippet}
    </States>
  {:else if status === 'error'}
    <States state="error" message="Could not load who runs mocks." onRetry={() => (attempt += 1)} />
  {:else if hosts.length === 0}
    {#if topic}
      <EmptyState
        class="py-8"
        variant="muted"
        title="Nobody hosts {mockTopicLabel(topic)} mocks with a free hour right now."
      >
        {#snippet action()}
          <Button variant="outline" size="sm" onclick={() => (topic = '')}>Show all topics</Button>
        {/snippet}
      </EmptyState>
    {:else if hosting}
      <EmptyState
        class="py-8"
        variant="muted"
        title="Nobody else has a free hour in the next 30 days. Members can still book you."
      />
    {:else}
      <EmptyState
        class="py-8"
        title="Nobody runs mocks yet — be the first"
        description="Save a meeting link and a few weekly hours below, and other members can book a practice hour with you."
      >
        {#snippet icon()}<UsersRound class="size-6" aria-hidden="true" />{/snippet}
        {#snippet action()}
          <Button variant="outline" size="sm" href={hostingAnchor}>Set up hosting</Button>
        {/snippet}
      </EmptyState>
    {/if}
  {:else}
    <ul class="-mx-2 flex flex-col divide-y divide-border">
      {#each hosts as host (host.handle)}
        {@const rating = hostRatingLine(host)}
        <li class="flex flex-col gap-3 px-2 py-3 md:flex-row md:items-center md:gap-4">
          <div class="min-w-0 md:w-56 md:shrink-0">
            <BuddyIdentity handle={host.handle} summary={host.card_summary} size="sm" />
          </div>
          <div class="flex flex-1 flex-wrap gap-1.5">
            {#each host.topics as t (t)}<Chip>{mockTopicLabel(t)}</Chip>{/each}
          </div>
          <div class="flex items-center justify-between gap-4 md:justify-end">
            <div class="flex flex-col md:w-32">
              <span class="text-xs text-muted-foreground">Next free</span>
              <span class="text-sm font-medium tabular-nums" title="UTC{host.next_slot.utc_offset}">
                {hostSlotLabel(host.next_slot.local_start)}
              </span>
            </div>
            <div class="flex text-sm tabular-nums md:w-28">
              {#if rating}
                <span class="flex items-center gap-1" title="Average of revealed ratings · completed mock interviews">
                  <Star class="size-3.5 fill-current text-foreground" aria-hidden="true" />
                  <span class="sr-only">Rated</span>{rating}
                </span>
              {/if}
            </div>
            <Button variant="outline" size="sm" href={bookHref(host.handle)} aria-label="Book a mock with {host.handle}">
              Book
            </Button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>
