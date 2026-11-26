<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import type { Availability } from '#lib/api-methods/availability.js';
  import { Button } from '#lib/ui/index.js';
  import { errorMessage } from '#lib/utils.js';
  import type { AvailabilityApi } from '#lib/types.js';
  import MentorScheduleEditor from './MentorScheduleEditor.svelte';

  // The interview scheduling agent's hours and rules, on the one calendar mentorship, mocks
  // and booking links share. With `onconnect` it is the consent step before Google: the user
  // reviews the hours and what the agent will do, then ticks the box to continue.
  let { onconnect }: { onconnect?: () => void } = $props();

  const DURATIONS = [15, 30, 45, 60];
  const NOTICES = [0, 2, 4, 12, 24, 48, 72, 168];

  let availability = $state<Availability | null>(null);
  let loadError = $state('');
  let duration = $state(30);
  let notice = $state(12);
  let savingBooking = $state(false);
  let bookingNote = $state('');
  let agreed = $state(false);

  // A first save states the browser's zone; after that the stored zone stays.
  const zone = $derived(availability?.timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone);

  const schedule: AvailabilityApi = {
    replaceWeek: (rows) => api.replaceAvailabilityWeek(rows, zone),
    reload: async () => (await api.myAvailability()).rules,
    addOverride: (date, start, end) => api.addAvailabilityDated(date, start, end, zone),
    remove: api.deleteAvailabilityEntry,
  };

  onMount(async () => {
    try {
      availability = await api.myAvailability();
      duration = availability.booking.duration_min;
      notice = availability.booking.notice_hours;
    } catch (e) {
      loadError = errorMessage(e, 'Could not load your hours.');
    }
  });

  async function saveBooking() {
    savingBooking = true;
    bookingNote = '';
    try {
      await api.saveAvailabilityBooking({ timezone: zone, duration_min: duration, notice_hours: notice });
      bookingNote = 'Saved.';
    } catch (e) {
      bookingNote = errorMessage(e, 'Could not save.');
    } finally {
      savingBooking = false;
    }
  }

  const noticeLabel = (h: number) => (h === 0 ? 'No notice' : h % 24 === 0 ? `${h / 24} d` : `${h} h`);
  const selectClass = 'border-input bg-background mt-1 w-full rounded-md border px-2 py-1.5 text-sm';
</script>

<div class="flex flex-col gap-4 border-t border-border pt-4">
  {#if loadError}
    <p class="text-xs text-destructive" role="alert">{loadError}</p>
  {:else if availability}
    <p class="text-xs text-muted-foreground">
      Times are in {zone}. These hours are shared with your mentorship, mock interviews and booking links, and a meeting
      in any of them blocks the same time in the others.
    </p>
    <MentorScheduleEditor
      bind:rules={availability.rules}
      {schedule}
      emptyMessage="No hours saved yet. Until you add some, the agent offers Monday to Friday, 10:00 to 18:00."
    />
    <div class="flex flex-wrap items-end gap-3">
      <label class="block text-sm">
        <span class="text-muted-foreground">Meeting length</span>
        <select bind:value={duration} class={selectClass}>
          {#each DURATIONS as d (d)}<option value={d}>{d} min</option>{/each}
        </select>
      </label>
      <label class="block text-sm">
        <span class="text-muted-foreground">Shortest notice</span>
        <select bind:value={notice} class={selectClass}>
          {#each NOTICES as h (h)}<option value={h}>{noticeLabel(h)}</option>{/each}
        </select>
      </label>
      <Button variant="outline" size="sm" disabled={savingBooking} onclick={saveBooking}>Save</Button>
      {#if bookingNote}<span class="text-xs text-muted-foreground" role="status">{bookingNote}</span>{/if}
    </div>

    {#if onconnect}
      <div class="flex flex-col gap-2 text-sm">
        <h4 class="font-medium">What the agent does</h4>
        <ul class="list-disc space-y-1 pl-5 text-muted-foreground">
          <li>
            Reads your Gmail for interview invitations about jobs you applied to, and answers only those that ask for a
            time and nothing else. Every other email stays with you.
          </li>
          <li>
            Offers three times from the hours above, minus busy time in your Google Calendar and your other Freehire
            meetings, plus your booking link.
          </li>
          <li>
            Replies from your Gmail in short fixed wording with only the times and the link. Never your salary, contacts
            or other details.
          </li>
          <li>When the recruiter picks a time, it adds a Google Meet event to your Calendar and invites them.</li>
          <li>If the recruiter goes quiet, it follows up at most twice.</li>
          <li>Sends you one email a day with what it did. You can turn it off here at any time.</li>
        </ul>
        <label class="flex items-start gap-2">
          <input type="checkbox" bind:checked={agreed} class="mt-0.5 size-4 accent-current" />
          <span>I have checked my hours and want Freehire to answer interview invitations for me.</span>
        </label>
        <div>
          <Button variant="primary" size="sm" disabled={!agreed} onclick={onconnect}>Continue with Google</Button>
        </div>
      </div>
    {/if}
  {:else}
    <p class="text-xs text-muted-foreground">Loading your hours…</p>
  {/if}
</div>
