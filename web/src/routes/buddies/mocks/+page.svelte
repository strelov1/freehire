<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import BuddyMockList from '#lib/components/BuddyMockList.svelte';
  import BuddyMockSettingsForm from '#lib/components/BuddyMockSettingsForm.svelte';
  import MentorScheduleEditor from '#lib/components/MentorScheduleEditor.svelte';
  import States from '#lib/components/States.svelte';
  import type { AvailabilityApi, BuddyMockSettings, BuddyMocks, MentorAvailabilityRule } from '#lib/types.js';

  // The caller's own mock interviews: what is coming, when they host, and what is past.
  // Booking happens on another member's page (/buddies/[handle]). The space's layout shows
  // the join screen to a non-member, so this page only ever renders for a member.

  const mockSchedule: AvailabilityApi = {
    replaceWeek: api.replaceBuddyMockWeekly,
    reload: api.myBuddyMockAvailability,
    addOverride: api.addBuddyMockOverride,
    remove: api.deleteBuddyMockAvailabilityRule,
  };

  let view = $state<'loading' | 'ready' | 'error'>('loading');
  let settings = $state<BuddyMockSettings | null>(null);
  let rules = $state<MentorAvailabilityRule[]>([]);
  let mocks = $state<BuddyMocks>({ upcoming: [], past: [] });

  async function refreshMocks() {
    mocks = await api.listBuddyMocks();
  }

  onMount(async () => {
    try {
      [settings, rules, mocks] = await Promise.all([
        api.getBuddyMockSettings(),
        api.myBuddyMockAvailability(),
        api.listBuddyMocks(),
      ]);
      view = 'ready';
    } catch {
      view = 'error';
    }
  });
</script>

<svelte:head><title>Mock interviews · Interview buddies</title></svelte:head>

<div class="flex max-w-4xl flex-col gap-5">
  <header class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold">Mock interviews</h1>
    <p class="text-muted-foreground text-sm">
      Practise with another member: sixty minutes on the host's own call link, booked at least
      twelve hours ahead. Members book you from your buddy page; you book them from theirs.
    </p>
  </header>

  {#if view === 'loading'}<States state="loading" />
  {:else if view === 'error'}<States state="error" />
  {:else}
    <section class="flex flex-col gap-3">
      <h2 class="text-lg font-medium">Upcoming</h2>
      {#if mocks.upcoming.length === 0}
        <p class="text-muted-foreground text-sm">No mock interviews booked.</p>
      {:else}
        <BuddyMockList mocks={mocks.upcoming} upcoming onchange={refreshMocks} />
      {/if}
    </section>

    <BuddyMockSettingsForm bind:settings />

    <section class="flex flex-col gap-3">
      <h2 class="text-lg font-medium">When you can host</h2>
      <p class="text-muted-foreground text-sm">Hours are in the time zone saved above.</p>
      <MentorScheduleEditor
        bind:rules
        schedule={mockSchedule}
        emptyMessage="No hours yet — until you add some, nobody can book a mock interview with you."
      />
    </section>

    {#if mocks.past.length > 0}
      <section class="flex flex-col gap-3">
        <h2 class="text-lg font-medium">Past</h2>
        <BuddyMockList mocks={mocks.past} upcoming={false} onchange={refreshMocks} />
      </section>
    {/if}
  {/if}
</div>
