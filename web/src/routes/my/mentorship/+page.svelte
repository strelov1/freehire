<script lang="ts">
  import MentorSessionList from '$lib/components/MentorSessionList.svelte';
  import { browserTimezone } from '$lib/mentorship';
  import { Button } from '$lib/ui';
  import { resolve } from '$app/paths';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const timezone = browserTimezone();
</script>

<div class="flex flex-col gap-6">
  <section class="flex flex-col gap-3">
    <MentorSessionList sessions={data.sessions.upcoming} {timezone} empty="No sessions booked." />
  </section>

  {#if data.sessions.past.length > 0}
    <section class="flex flex-col gap-3">
      <h2 class="text-muted-foreground text-sm">Past</h2>
      <!-- A cancelled session is here whatever its clock says: nobody is going to it. -->
      <MentorSessionList sessions={data.sessions.past} {timezone} empty="Nothing yet." />
    </section>
  {/if}

  {#if !data.profile}
    <!-- Only for somebody who is not a mentor. Once they are, this is the Profile tab and
         an invitation to do what they have already done reads as a bug. -->
    <section class="flex flex-col items-start gap-3 border-t pt-6">
      <div>
        <h2 class="text-sm font-medium">Mentor at your company?</h2>
        <p class="text-muted-foreground mt-1 text-sm">
          Publish a profile and take bookings. A moderator reviews it first.
        </p>
      </div>
      <Button variant="outline" href={resolve('/my/mentorship/profile')}>Offer to mentor</Button>
    </section>
  {/if}
</div>
