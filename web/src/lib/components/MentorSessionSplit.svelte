<script lang="ts">
  import MentorSessionList from './MentorSessionList.svelte';
  import { browserTimezone } from '$lib/mentorship';
  import type { MentorSessions } from '$lib/types';

  // One party's sessions, both halves. The two panes that show them — the seeker's own and
  // the mentor's incoming — differ only in the data and in what an empty list should say,
  // so the shape and the reason behind it live here once.
  let { sessions, empty }: { sessions: MentorSessions; empty: string } = $props();

  const timezone = browserTimezone();
</script>

<div class="flex flex-col gap-6">
  <section class="flex flex-col gap-3">
    <MentorSessionList sessions={sessions.upcoming} {timezone} {empty} />
  </section>

  {#if sessions.past.length > 0}
    <section class="flex flex-col gap-3">
      <h2 class="text-muted-foreground text-sm">Past</h2>
      <!-- A cancelled session is here whatever its clock says: nobody is going to it. The
           split is the SERVER's, made against one clock for the whole list, so a session
           starting mid-read cannot land in both halves or in neither. -->
      <MentorSessionList sessions={sessions.past} {timezone} empty="Nothing yet." />
    </section>
  {/if}
</div>
