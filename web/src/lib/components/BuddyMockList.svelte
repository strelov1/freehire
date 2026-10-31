<script lang="ts">
  import { ExternalLink, Star } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Chip, ConfirmDialog } from '#lib/ui/index.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { mockCounterpartLabel, mockDateParts, mockRoleLabel, mockTopicLabel } from '#lib/buddyMocks.js';
  import BuddyAvatar from './BuddyAvatar.svelte';
  import BuddyMockReviewForm from './BuddyMockReviewForm.svelte';
  import type { BuddyMock } from '#lib/types.js';

  // The caller's own mocks as rows: a date tile, the hours in the viewer's zone, the other
  // member, the topic and role, and what can still be done — join, cancel, review. The
  // host draws the frame; rows are separated, not boxed, so nothing nests a card.
  let {
    mocks,
    upcoming,
    onchange,
  }: { mocks: BuddyMock[]; upcoming: boolean; onchange: () => Promise<void> } = $props();

  const zone = browserTimezone();
  let cancelling = $state<BuddyMock | null>(null);
  let confirmOpen = $state(false);
  let error = $state('');

  function askCancel(mock: BuddyMock) {
    cancelling = mock;
    confirmOpen = true;
  }

  async function cancel() {
    if (!cancelling) return;
    error = '';
    try {
      await api.cancelBuddyMock(cancelling.id);
      await onchange();
    } catch (e) {
      error = errorMessage(e, 'The mock interview could not be cancelled.');
    } finally {
      cancelling = null;
    }
  }
</script>

{#snippet stars(rating: number)}
  <span class="inline-flex items-center gap-0.5" role="img" aria-label="{rating} out of 5">
    {#each [1, 2, 3, 4, 5] as n (n)}
      <Star class="size-3.5 {n <= rating ? 'fill-current text-foreground' : 'text-muted-foreground/40'}" aria-hidden="true" />
    {/each}
  </span>
{/snippet}

{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}

<ul class="flex flex-col divide-y divide-border">
  {#each mocks as mock (mock.id)}
    {@const when = mockDateParts(mock, zone)}
    {@const cancelled = mock.status === 'cancelled'}
    <li class="flex flex-col gap-3 py-4 first:pt-0 last:pb-0">
      <div class="flex gap-4">
        <div
          class="flex w-14 shrink-0 flex-col items-center rounded-lg py-1.5 leading-none {cancelled
            ? 'bg-muted text-muted-foreground'
            : 'bg-brand-muted text-brand-strong'}"
          aria-hidden="true"
        >
          <span class="text-xs font-medium uppercase">{when.weekday}</span>
          <span class="mt-1 text-xl font-semibold tabular-nums">{when.day}</span>
          <span class="mt-0.5 text-xs">{when.month}</span>
        </div>

        <div class="flex min-w-0 flex-1 flex-col gap-1.5">
          <p class="text-sm">
            <span class="font-semibold tabular-nums {cancelled ? 'text-muted-foreground line-through' : ''}">
              <span class="sr-only">{when.weekday} {when.day} {when.month}, </span>{when.time}
            </span>
            <span class="text-muted-foreground"> · {zone}</span>
          </p>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-sm">
            {#if mock.counterpart}
              <a
                href={resolve('/buddies/[handle]', { handle: mock.counterpart })}
                class="flex min-w-0 items-center gap-2 font-medium hover:underline"
              >
                <BuddyAvatar handle={mock.counterpart} size="sm" class="shrink-0" />
                <span class="truncate">{mock.counterpart}</span>
              </a>
            {:else}
              <span class="text-muted-foreground">{mockCounterpartLabel(mock)}</span>
            {/if}
            <span class="flex flex-wrap items-center gap-1.5">
              <Chip>{mockTopicLabel(mock.topic)}</Chip>
              <Badge variant="outline">{mockRoleLabel(mock)}</Badge>
              {#if cancelled}<Badge variant="missing">Cancelled</Badge>{/if}
            </span>
          </div>
        </div>

        {#if (upcoming && mock.status === 'confirmed' && mock.meeting_url) || mock.can_cancel}
          <div class="hidden shrink-0 items-start gap-2 sm:flex">
            {@render actions(mock)}
          </div>
        {/if}
      </div>

      {#if (upcoming && mock.status === 'confirmed' && mock.meeting_url) || mock.can_cancel}
        <div class="flex gap-2 sm:hidden">{@render actions(mock)}</div>
      {/if}

      {#if mock.their_review || mock.my_review}
        <dl class="flex flex-col gap-1 text-sm sm:pl-18">
          {#if mock.their_review}
            <div class="flex flex-wrap items-center gap-x-2">
              <dt class="text-muted-foreground">Their rating</dt>
              <dd class="flex items-center gap-2">
                {@render stars(mock.their_review.rating)}
                {#if mock.their_review.comment}<span>{mock.their_review.comment}</span>{/if}
              </dd>
            </div>
          {/if}
          {#if mock.my_review}
            <div class="flex flex-wrap items-center gap-x-2">
              <dt class="text-muted-foreground">You rated them</dt>
              <dd>{@render stars(mock.my_review.rating)}</dd>
            </div>
          {/if}
        </dl>
      {/if}
      {#if mock.can_review}
        <div class="sm:pl-18"><BuddyMockReviewForm id={mock.id} onreviewed={onchange} /></div>
      {/if}
    </li>
  {/each}
</ul>

{#snippet actions(mock: BuddyMock)}
  {#if upcoming && mock.status === 'confirmed' && mock.meeting_url}
    <Button variant="primary" size="sm" href={mock.meeting_url} target="_blank" rel="noreferrer">
      Join call <ExternalLink class="size-3.5" aria-hidden="true" />
    </Button>
  {/if}
  {#if mock.can_cancel}
    <Button variant="outline" size="sm" onclick={() => askCancel(mock)}>Cancel</Button>
  {/if}
{/snippet}

<ConfirmDialog
  bind:open={confirmOpen}
  title="Cancel this mock interview?"
  description="The other member is told it was cancelled, and the time is offered again."
  confirmLabel="Cancel it"
  onConfirm={cancel}
/>
