<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Card, ConfirmDialog } from '#lib/ui/index.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { mockCounterpartLabel, mockRoleLabel, mockTopicLabel, mockWhen } from '#lib/buddyMocks.js';
  import BuddyMockReviewForm from './BuddyMockReviewForm.svelte';
  import type { BuddyMock } from '#lib/types.js';

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

{#if error}<p role="alert" class="text-destructive text-sm">{error}</p>{/if}

<ul class="flex flex-col gap-3">
  {#each mocks as mock (mock.id)}
    <li>
      <Card class="flex flex-col gap-2 p-4">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span class="font-medium">{mockCounterpartLabel(mock)}</span>
          <Badge variant="secondary">{mockTopicLabel(mock.topic)}</Badge>
          <span class="text-muted-foreground">{mockRoleLabel(mock)}</span>
          {#if mock.status === 'cancelled'}<Badge variant="outline">Cancelled</Badge>{/if}
        </div>
        <p class="text-sm">{mockWhen(mock, zone)} <span class="text-muted-foreground">({zone})</span></p>

        {#if upcoming && mock.status === 'confirmed' && mock.meeting_url}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the host's own meeting room, an external URL, not an internal route -->
          <a class="w-fit text-sm font-medium underline" href={mock.meeting_url} rel="noreferrer" target="_blank">Join the call</a>
        {/if}
        {#if mock.can_cancel}
          <div><Button variant="ghost" size="sm" onclick={() => askCancel(mock)}>Cancel</Button></div>
        {/if}

        {#if mock.their_review}
          <p class="text-sm">
            Their rating: {mock.their_review.rating} ★{#if mock.their_review.comment} — {mock.their_review.comment}{/if}
          </p>
        {/if}
        {#if mock.my_review}
          <p class="text-muted-foreground text-sm">You rated them {mock.my_review.rating} ★</p>
        {/if}
        {#if mock.can_review}
          <BuddyMockReviewForm id={mock.id} onreviewed={onchange} />
        {/if}
      </Card>
    </li>
  {/each}
</ul>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Cancel this mock interview?"
  description="The other member is told it was cancelled, and the time is offered again."
  confirmLabel="Cancel it"
  onConfirm={cancel}
/>
