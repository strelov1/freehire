<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/utils';
  import { browserTimezone, canReview, formatInstantIn, isCancellable } from '$lib/mentorship';
  import { Badge, Button, Card } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const session = $derived(data.session);
  const timezone = browserTimezone();
  const starts = $derived(formatInstantIn(session.starts_at, timezone));
  const ends = $derived(formatInstantIn(session.ends_at, timezone));

  // The button is hidden for a past or already-cancelled session rather than shown and
  // refused: the endpoint would reject it either way, and a control whose only outcome is
  // an error is worse than no control.
  const cancellable = $derived(isCancellable(session));

  let reason = $state('');
  let cancelling = $state(false);
  let cancelError = $state('');
  let confirming = $state(false);

  // The review form appears only for the seeker of a session that actually happened.
  const reviewable = $derived(canReview(session));
  let rating = $state(5);
  let comment = $state('');
  let reviewing = $state(false);
  let reviewError = $state('');
  let reviewed = $state(false);

  async function submitReview() {
    reviewing = true;
    reviewError = '';
    try {
      await api.reviewMySession(session.id, rating, comment);
      // PUT, so a second submission replaces the first rather than adding one — the
      // mentor's review count does not move. Nothing to re-read: the session itself is
      // unchanged by the review.
      reviewed = true;
    } catch (e) {
      reviewError = errorMessage(e, 'The review could not be saved.');
    } finally {
      reviewing = false;
    }
  }

  async function cancel() {
    cancelling = true;
    cancelError = '';
    try {
      await api.cancelMySession(session.id, reason);
      // Re-read rather than patching the row here: cancellation also decides which half of
      // the list this lands in, and that split is the server's to make.
      await invalidateAll();
      confirming = false;
    } catch (e) {
      cancelError = errorMessage(e, 'The session could not be cancelled.');
    } finally {
      cancelling = false;
    }
  }
</script>

<div class="flex max-w-2xl flex-col gap-6">
  <div>
    <a
      href={resolve('/my/mentorship')}
      class="text-muted-foreground hover:text-foreground text-sm">← All sessions</a
    >
    <h1 class="mt-2 text-xl font-semibold">{starts.day} at {starts.time}–{ends.time}</h1>
    <p class="text-muted-foreground mt-1 text-sm">
      {session.headline} · UTC{starts.offset} · times shown in {timezone}
    </p>
  </div>

  {#if session.status !== 'confirmed'}
    <Badge variant="secondary">{session.status}</Badge>
  {/if}

  <Card class="flex flex-col gap-3 p-4">
    {#if session.status === 'confirmed'}
      <!-- The link is the whole point of the booking, and it is a live room: it reaches
           the two parties here and appears on no public page. -->
      <p class="text-sm">
        <span class="text-muted-foreground">Meeting link</span><br />
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the mentor's own meeting room, an external URL, not an internal route -->
        <a class="underline" href={session.meeting_url} rel="noreferrer" target="_blank"
          >{session.meeting_url}</a
        >
      </p>
    {/if}

    {#if session.seeker_email}
      <p class="text-sm">
        <span class="text-muted-foreground">Booked by</span><br />{session.seeker_email}
      </p>
    {/if}

    {#if session.note}
      <p class="text-sm">
        <span class="text-muted-foreground">Note</span><br />{session.note}
      </p>
    {/if}
  </Card>

  {#if reviewable}
    <Card class="flex flex-col gap-3 p-4">
      <p class="text-sm font-medium">How was it?</p>
      {#if reviewed}
        <p class="text-muted-foreground text-sm">
          Saved. Sending it again replaces this one rather than adding another.
        </p>
      {/if}
      <label class="text-sm">
        <span class="text-muted-foreground">Rating</span>
        <select
          bind:value={rating}
          class="border-input bg-background mt-1 block h-9 rounded-md border px-2 text-sm"
        >
          {#each [5, 4, 3, 2, 1] as value (value)}
            <option {value}>{value}</option>
          {/each}
        </select>
      </label>
      <label class="text-sm">
        <span class="text-muted-foreground">Comment (optional)</span>
        <textarea
          bind:value={comment}
          rows="3"
          maxlength="1000"
          class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
        ></textarea>
      </label>
      <div>
        <Button disabled={reviewing} onclick={submitReview}>
          {reviewing ? 'Saving…' : reviewed ? 'Update the review' : 'Leave a review'}
        </Button>
      </div>
      {#if reviewError}
        <p class="text-destructive text-sm">{reviewError}</p>
      {/if}
    </Card>
  {/if}

  {#if cancellable}
    {#if confirming}
      <Card class="flex flex-col gap-3 p-4">
        <p class="text-sm font-medium">Cancel this session?</p>
        <p class="text-muted-foreground text-sm">
          The other side is told, and the hour goes back on the calendar.
        </p>
        <label class="block text-sm">
          <span class="text-muted-foreground">Reason (optional)</span>
          <input
            bind:value={reason}
            maxlength="500"
            class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
          />
        </label>
        <div class="flex gap-2">
          <Button variant="destructive" disabled={cancelling} onclick={cancel}>
            {cancelling ? 'Cancelling…' : 'Cancel the session'}
          </Button>
          <Button variant="ghost" disabled={cancelling} onclick={() => (confirming = false)}>
            Keep it
          </Button>
        </div>
        {#if cancelError}
          <p class="text-destructive text-sm">{cancelError}</p>
        {/if}
      </Card>
    {:else}
      <div>
        <Button variant="outline" onclick={() => (confirming = true)}>Cancel session</Button>
      </div>
    {/if}
  {/if}
</div>
