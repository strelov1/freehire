<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button } from '#lib/ui/index.js';
  import MentorBookingView from './MentorBookingView.svelte';
  import { MOCK_DURATION_MINUTES, mockTopicLabel, reviewSummaryLine } from '#lib/buddyMocks.js';
  import type { BuddyMockSummary, BuddyMockTopic, MentorSlot, MentorSlots } from '#lib/types.js';

  let { handle }: { handle: string } = $props();

  let summary = $state<BuddyMockSummary | null>(null);
  let topics = $state<BuddyMockTopic[] | null>(null);
  let topic = $state<BuddyMockTopic | ''>('');
  let booking = $state(false);
  let bookingError = $state('');

  $effect(() => {
    const current = handle;
    let cancelled = false;
    summary = null;
    topics = null;
    topic = '';
    api.getBuddyMockSummary(current).then(
      (s) => {
        if (!cancelled) summary = s;
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  });

  // The slot read also says which topics this member hosts; an empty list is a member who
  // is not hosting (no settings, or paused).
  async function loadSlots(from: string, to: string, timezone: string, signal: AbortSignal): Promise<MentorSlots> {
    const res = await api.getBuddyMockSlots(handle, from, to, timezone, signal);
    topics = res.topics;
    if (topic && !res.topics.includes(topic)) topic = '';
    return { slots: res.slots, timezone: res.timezone };
  }

  async function book(slot: MentorSlot, zone: string, refused: () => void) {
    if (!topic) return;
    booking = true;
    bookingError = '';
    try {
      // The absolute instant and the zone the server answered in, as a mentor booking sends.
      await api.bookBuddyMock(handle, { starts_at: slot.starts_at, topic, timezone: zone });
      void goto(resolve('/buddies/mocks'));
    } catch (e) {
      bookingError = errorMessage(e, 'That time could not be booked.');
      refused();
    } finally {
      booking = false;
    }
  }
</script>

<section class="flex flex-col gap-3">
  <h2 class="text-sm font-medium">Mock interview</h2>
  {#if summary}
    <p class="text-muted-foreground text-sm">{reviewSummaryLine(summary)}</p>
    {#if summary.recent.length > 0}
      <ul class="flex flex-col gap-1 text-sm">
        {#each summary.recent as review, i (i)}
          <li>
            <span class="font-medium">{review.rating} ★</span>
            <span class="text-muted-foreground">· {mockTopicLabel(review.topic)} · {review.day}</span>
            {#if review.comment}<span> — {review.comment}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  {#if topics !== null && topics.length === 0}
    <p class="text-muted-foreground text-sm">This member is not hosting mock interviews right now.</p>
  {:else}
    <MentorBookingView {loadSlots} sessionMinutes={MOCK_DURATION_MINUTES} counterpart={handle}>
      {#snippet booker({ slot, zone, refused })}
        <label class="mt-3 block text-sm">
          <span class="text-muted-foreground">Topic</span>
          <select bind:value={topic} class="border-input bg-background mt-1 h-9 w-full rounded-md border px-2 text-sm">
            <option value="" disabled>Choose a topic</option>
            {#each topics ?? [] as t (t)}
              <option value={t}>{mockTopicLabel(t)}</option>
            {/each}
          </select>
        </label>
        <p class="text-muted-foreground mt-2 text-xs">
          You get the meeting link once booked. Sixty minutes; swap roles halfway.
        </p>
        <Button variant="primary" class="mt-3" disabled={booking || !topic} onclick={() => book(slot, zone, refused)}>
          {booking ? 'Booking…' : 'Book this mock interview'}
        </Button>
        {#if bookingError}<p class="text-destructive mt-2 text-sm">{bookingError}</p>{/if}
      {/snippet}
    </MentorBookingView>
  {/if}
</section>
