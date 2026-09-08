<script lang="ts">
  import MentorSessionList from '$lib/components/MentorSessionList.svelte';
  import { browserTimezone } from '$lib/mentorship';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const timezone = browserTimezone();
</script>

<div class="flex flex-col gap-6">
  <section class="flex flex-col gap-3">
    <MentorSessionList
      sessions={data.bookings.upcoming}
      {timezone}
      empty="Nobody has booked yet."
    />
  </section>

  {#if data.bookings.past.length > 0}
    <section class="flex flex-col gap-3">
      <h2 class="text-muted-foreground text-sm">Past</h2>
      <MentorSessionList sessions={data.bookings.past} {timezone} empty="Nothing yet." />
    </section>
  {/if}
</div>
