<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';
  import type { Mentor, MentorSlot } from '#lib/types.js';

  // Taking one of a mentor's hours, rendered by MentorBookingView for the chosen slot.
  // `chosen`, not `slot`: a component attribute named `slot` is Svelte's legacy slot syntax.
  let {
    mentor,
    chosen,
    zone,
    refused,
  }: { mentor: Mentor; chosen: MentorSlot; zone: string; refused: () => void } = $props();

  const signedIn = $derived(Boolean(page.data.user));

  let note = $state('');
  let booking = $state(false);
  let bookingError = $state('');

  // Not promptSignIn(): that reads `page.url.search`, which lags our shallow writes, so a
  // visitor would return from signing in with the slot no longer chosen — the one thing
  // putting it in the URL was for. Read the address bar, which is current.
  function signInAndComeBack() {
    const returnTo = location.pathname + location.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo, mode: 'login' }));
  }

  async function book() {
    booking = true;
    bookingError = '';
    try {
      const session = await api.bookMentorSession(mentor.slug, {
        // The absolute instant, never the wall clock: this is what the server re-derives
        // the slot from.
        starts_at: chosen.starts_at,
        // Recorded on the booking so the confirmation is written in the zone the seeker
        // actually booked in — `zone`, which is what the server used, not what we asked.
        timezone: zone,
        note,
      });
      void goto(resolve('/my/mentorship/sessions/[id]', { id: session.id }));
    } catch (e) {
      // The hour may simply have gone: somebody else took it, or the mentor moved their
      // availability inside the minute the slot cache holds. Re-ask rather than leaving a
      // list that still offers it.
      bookingError = errorMessage(e, 'That hour could not be booked.');
      refused();
    } finally {
      booking = false;
    }
  }
</script>

{#if signedIn}
  <label class="mt-3 block text-sm">
    <span class="text-muted-foreground">What would you like to talk about? (optional)</span>
    <textarea
      bind:value={note}
      rows="3"
      maxlength="1000"
      class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      placeholder="A sentence is plenty — it helps them prepare."
    ></textarea>
  </label>

  <Button class="mt-3" disabled={booking} onclick={book}>
    {booking ? 'Booking…' : 'Book this session'}
  </Button>
{:else}
  <p class="text-muted-foreground mt-3 text-sm">
    Sign in to book. Your chosen hour is in the address, so it will still be here.
  </p>
  <Button class="mt-3" onclick={signInAndComeBack}>Sign in to book</Button>
{/if}

{#if bookingError}
  <p class="text-destructive mt-2 text-sm">{bookingError}</p>
{/if}
