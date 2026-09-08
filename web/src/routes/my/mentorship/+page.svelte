<script lang="ts">
  import MentorProfileEditor from '$lib/components/MentorProfileEditor.svelte';
  import MentorScheduleEditor from '$lib/components/MentorScheduleEditor.svelte';
  import MentorSessionList from '$lib/components/MentorSessionList.svelte';
  import { Button } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // The zone to read the instants in. Resolved here rather than taken from a session: a
  // booking records the zone it was MADE in, which is not necessarily the one the person
  // is sitting in now.
  const timezone =
    typeof Intl === 'undefined' ? 'UTC' : Intl.DateTimeFormat().resolvedOptions().timeZone;

  // Local copies so the editors can write back without a round trip through `load`.
  let profile = $state(data.profile);
  let availability = $state(data.availability);

  // Offering to mentor is a deliberate act, not a tab somebody lands in: the form only
  // appears once asked for, so a seeker reading their own sessions is not invited to fill
  // in a company slug they have no reason to think about.
  let offering = $state(false);
</script>

<div class="flex flex-col gap-8">
  <div>
    <h1 class="text-xl font-semibold">Mentorship</h1>
    <p class="text-muted-foreground mt-1 text-sm">Times shown in {timezone}.</p>
  </div>

  <section class="flex flex-col gap-3">
    <h2 class="text-sm font-medium">Sessions you booked</h2>
    <MentorSessionList
      sessions={data.sessions.upcoming}
      {timezone}
      empty="No sessions booked."
    />
    {#if data.sessions.past.length > 0}
      <h3 class="text-muted-foreground mt-2 text-sm">Past</h3>
      <!-- A cancelled session is here whatever its clock says: nobody is going to it. -->
      <MentorSessionList sessions={data.sessions.past} {timezone} empty="Nothing yet." />
    {/if}
  </section>

  {#if profile}
    <section class="flex flex-col gap-3 border-t pt-6">
      <h2 class="text-sm font-medium">Sessions booked with you</h2>
      {#if data.bookings}
        <MentorSessionList
          sessions={data.bookings.upcoming}
          {timezone}
          empty="Nobody has booked yet."
        />
      {/if}
    </section>

    <section class="flex flex-col gap-3 border-t pt-6">
      <MentorProfileEditor bind:profile />
    </section>

    <section class="flex flex-col gap-3">
      <MentorScheduleEditor bind:rules={availability} />
    </section>
  {:else if offering}
    <section class="flex flex-col gap-3 border-t pt-6">
      <MentorProfileEditor bind:profile />
    </section>
  {:else}
    <section class="flex flex-col items-start gap-3 border-t pt-6">
      <div>
        <h2 class="text-sm font-medium">Mentor at your company?</h2>
        <p class="text-muted-foreground mt-1 text-sm">
          Publish a profile and take bookings. A moderator reviews it first.
        </p>
      </div>
      <Button variant="outline" onclick={() => (offering = true)}>Offer to mentor</Button>
    </section>
  {/if}
</div>
