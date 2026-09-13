<script lang="ts">
  import MentorScheduleCalendar from '$lib/components/MentorScheduleCalendar.svelte';
  import MentorScheduleEditor from '$lib/components/MentorScheduleEditor.svelte';
  import MentorSessionSettings from '$lib/components/MentorSessionSettings.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // Local copies so the editors can write back without a round trip through `load`.
  let availability = $state(data.availability);
  let profile = $state(data.profile);
</script>

<div class="flex flex-col gap-4">
  <!-- This route's own load calls requireMentorProfile, which redirects away before
       this component ever renders for an account with none — the guard below is
       purely to satisfy the type, which the layout still states as nullable. -->
  {#if profile}
    <MentorSessionSettings bind:profile />
  {/if}
  <!-- Read-only view of what the rules below resolve to, once bookings and synced
       calendar time are taken into account. Editing still happens in the form. -->
  <MentorScheduleCalendar calendar={data.calendar} />
  <MentorScheduleEditor bind:rules={availability} />
</div>
