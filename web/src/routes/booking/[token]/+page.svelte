<script lang="ts">
  import { api, ApiError } from '#lib/api.js';
  import MentorBookingView from '#lib/components/MentorBookingView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { slotLocalTime } from '#lib/mentorship.js';
  import { Button } from '#lib/ui/index.js';
  import { errorMessage } from '#lib/utils.js';
  import type { MentorSlot } from '#lib/types.js';
  import type { PageData } from './$types';

  // A recruiter books a time with a candidate from the link the candidate shared. No account:
  // a name and an address are all the candidate sees about who booked.
  let { data }: { data: PageData } = $props();

  let name = $state('');
  let email = $state('');
  let booking = $state(false);
  let bookingError = $state('');
  let booked = $state<{ slot: MentorSlot; day: string } | null>(null);

  async function book(slot: MentorSlot, day: string, refused: () => void) {
    booking = true;
    bookingError = '';
    try {
      await api.bookBookingSlot(data.token, { starts_at: slot.starts_at, name, email });
      booked = { slot, day };
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        bookingError = 'This time is no longer available. Please pick another.';
        refused();
      } else {
        bookingError = errorMessage(e, 'That time could not be booked.');
      }
    } finally {
      booking = false;
    }
  }
</script>

<Seo title="Book a time · freehire" description="Pick a time for a call." robots="noindex, nofollow" />

<div class="mx-auto flex w-full max-w-4xl flex-col gap-6 px-4 py-6">
  <header class="flex flex-col gap-2">
    <h1 class="text-2xl font-semibold">Book a time</h1>
    <p class="text-muted-foreground">Pick a 30-minute slot for a call with the candidate. No account needed.</p>
  </header>

  {#if booked}
    <p role="status" class="text-sm">
      Booked for {booked.day} at {slotLocalTime(booked.slot)}. The candidate can see it now.
    </p>
  {:else}
    <MentorBookingView
      loadSlots={(_from, _to, timezone, signal) => api.getBookingSlots(data.token, timezone, signal)}
      sessionMinutes={30}
      counterpart="the candidate"
    >
      {#snippet booker({ slot, day, refused })}
        <form
          class="mt-3 flex flex-col gap-3"
          onsubmit={(e) => {
            e.preventDefault();
            void book(slot, day, refused);
          }}
        >
          <label class="block text-sm">
            <span class="text-muted-foreground">Your name</span>
            <input
              bind:value={name}
              required
              maxlength="200"
              autocomplete="name"
              class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <label class="block text-sm">
            <span class="text-muted-foreground">Your work email</span>
            <input
              bind:value={email}
              type="email"
              required
              maxlength="254"
              autocomplete="email"
              class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <Button type="submit" disabled={booking}>{booking ? 'Booking…' : 'Book this time'}</Button>
          {#if bookingError}
            <p class="text-destructive text-sm">{bookingError}</p>
          {/if}
        </form>
      {/snippet}
    </MentorBookingView>
  {/if}
</div>
