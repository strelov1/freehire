<script lang="ts">
  import MentorSessionList from '$lib/components/MentorSessionList.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // The zone to read the instants in. Resolved here rather than taken from a session:
  // a booking records the zone it was MADE in, which is not necessarily the one the
  // person is sitting in now.
  const timezone =
    typeof Intl === 'undefined' ? 'UTC' : Intl.DateTimeFormat().resolvedOptions().timeZone;
</script>

<div class="flex flex-col gap-8">
  <div>
    <h1 class="text-xl font-semibold">Mentorship sessions</h1>
    <p class="text-muted-foreground mt-1 text-sm">Times shown in {timezone}.</p>
  </div>

  <section class="flex flex-col gap-3">
    <h2 class="text-sm font-medium">Upcoming</h2>
    <MentorSessionList
      sessions={data.sessions.upcoming}
      {timezone}
      empty="No sessions booked."
    />
  </section>

  <section class="flex flex-col gap-3">
    <h2 class="text-sm font-medium">Past</h2>
    <!-- A cancelled session is here whatever its clock says: nobody is going to it. -->
    <MentorSessionList sessions={data.sessions.past} {timezone} empty="Nothing yet." />
  </section>
</div>
