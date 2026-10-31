<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import BuddyMockHosts from '#lib/components/BuddyMockHosts.svelte';
  import BuddyMockList from '#lib/components/BuddyMockList.svelte';
  import BuddyMockSettingsForm from '#lib/components/BuddyMockSettingsForm.svelte';
  import MentorScheduleEditor from '#lib/components/MentorScheduleEditor.svelte';
  import States from '#lib/components/States.svelte';
  import type { AvailabilityApi, BuddyMockSettings, BuddyMocks, MentorAvailabilityRule } from '#lib/types.js';

  // Mock interviews: who the caller could book now, what they have coming, how they host,
  // and what is past. Booking happens on the host's page (/buddies/[handle], mock tab). The
  // space's layout shows the join screen to a non-member, so this page only ever renders
  // for a member.

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

<div class="flex max-w-4xl flex-col gap-4">
  <header class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold">Mock interviews</h1>
    <p class="text-sm text-muted-foreground">
      Practise with another member: sixty minutes on the host's own call link, booked at least
      twelve hours ahead. Book someone below; members book you from your buddy page.
    </p>
  </header>

  <BuddyMockHosts hostingAnchor="#hosting" hosting={!!settings && !settings.paused} />

  {#if view === 'loading'}<States state="loading" rows={3} />
  {:else if view === 'error'}<States state="error" message="Could not load your mock interviews." />
  {:else}
    <section class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5">
      <h2 class="text-base font-semibold">Your upcoming mocks</h2>
      {#if mocks.upcoming.length === 0}
        <p class="text-sm text-muted-foreground">Nothing booked. Pick a host above, or wait for a member to book you.</p>
      {:else}
        <BuddyMockList mocks={mocks.upcoming} upcoming onchange={refreshMocks} />
      {/if}
    </section>

    <div id="hosting" class="flex scroll-mt-20 flex-col gap-4">
      <BuddyMockSettingsForm bind:settings />
      <MentorScheduleEditor
        bind:rules
        schedule={mockSchedule}
        emptyMessage="No hours yet — until you add some, nobody can book a mock interview with you."
      />
    </div>

    {#if mocks.past.length > 0}
      <section class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5">
        <h2 class="text-base font-semibold">Past mocks</h2>
        <BuddyMockList mocks={mocks.past} upcoming={false} onchange={refreshMocks} />
      </section>
    {/if}
  {/if}
</div>
