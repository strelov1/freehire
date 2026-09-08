<script lang="ts">
  import MentorSessionSplit from '$lib/components/MentorSessionSplit.svelte';
  import { Button } from '$lib/ui';
  import { resolve } from '$app/paths';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<div class="flex flex-col gap-6">
  <MentorSessionSplit sessions={data.sessions} empty="No sessions booked." />

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
