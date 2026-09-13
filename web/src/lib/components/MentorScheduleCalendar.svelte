<script lang="ts">
  // Read-only, deliberately: this shows what the mentor's declared hours, confirmed
  // bookings and synced Google Calendar time resolve to — the same computation the
  // public booking page runs — but editing availability still goes through
  // MentorScheduleEditor's form below it. See the mentorship-schedule-calendar OpenSpec
  // change for why a second, calendar-shaped editor is out of scope here.
  import { api } from '$lib/api';
  import { Badge, Card, Skeleton } from '$lib/ui';
  import {
    addMonths,
    dayStatuses,
    groupCalendarByLocalDay,
    monthGrid,
    monthOf,
    todayIn,
    weekdayOrder,
    weekdayShortLabel,
  } from '$lib/mentorship';
  import type { MentorCalendar, MentorCalendarInterval } from '$lib/types';

  let { calendar: initial }: { calendar: MentorCalendar } = $props();

  let calendar = $state(initial);
  let month = $state(monthOf(todayIn(initial.timezone)));
  let loading = $state(false);
  let failed = $state(false);
  let selectedDay = $state('');

  const byDay = $derived(groupCalendarByLocalDay(calendar.intervals, calendar.timezone));
  const weeks = $derived(monthGrid(month));
  const dayIntervals = $derived(byDay.get(selectedDay) ?? []);

  async function load(target: string) {
    loading = true;
    try {
      calendar = await api.myMentorCalendar(target);
      failed = false;
    } catch {
      // Clear the stale month rather than leaving it on screen: `month` has already
      // moved, so keeping the previous fetch's intervals would show a grid full of
      // dots that belong to a different month right beside the failure message.
      calendar = { intervals: [], timezone: calendar.timezone };
      failed = true;
    } finally {
      loading = false;
    }
  }

  function stepMonth(delta: number) {
    month = addMonths(month, delta);
    selectedDay = '';
    void load(month);
  }

  const STATUS_LABEL: Record<MentorCalendarInterval['status'], string> = {
    booked: 'Booked',
    busy: 'Busy',
    free: 'Free',
    closed: 'Closed',
  };
  // Same priority order dayStatuses reports in — booked over busy over free over closed —
  // so the dots read left-to-right the way the underlying partition prioritises them.
  // Design-system tokens only (no raw Tailwind palette utilities, which
  // check-token-coverage gates): `warning` reads as "needs attention" for busy time,
  // and `brand` (an olive green) is the closest existing token to a positive/available
  // signal for free time — this palette has no dedicated success/green token yet.
  const STATUS_DOT: Record<MentorCalendarInterval['status'], string> = {
    booked: 'bg-primary',
    busy: 'bg-warning',
    free: 'bg-brand',
    closed: 'bg-muted-foreground/30',
  };

  const WEEKDAYS = weekdayOrder().map(weekdayShortLabel);
  const monthLabel = $derived(
    new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
      new Date(`${month}-01T00:00:00Z`),
    ),
  );

  // Full date and time, not just a wall clock: an interval spanning several days (a
  // mentor with no availability rules yet gets back one `closed` block for the whole
  // window) would otherwise print a start and end that both read "00:00".
  function fmtInstant(instant: string): string {
    return new Intl.DateTimeFormat('en-US', {
      timeZone: calendar.timezone,
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }).format(new Date(instant));
  }
</script>

<Card class="p-4">
  <div class="mb-2 flex items-center justify-between">
    <button
      type="button"
      class="hover:bg-muted rounded px-2 py-1 text-sm"
      onclick={() => stepMonth(-1)}
      aria-label="Previous month">‹</button
    >
    <span class="text-sm font-medium">{monthLabel}</span>
    <button
      type="button"
      class="hover:bg-muted rounded px-2 py-1 text-sm"
      onclick={() => stepMonth(1)}
      aria-label="Next month">›</button
    >
  </div>

  <div class="text-muted-foreground grid grid-cols-7 gap-1 text-center text-xs">
    {#each WEEKDAYS as weekday (weekday)}
      <span>{weekday}</span>
    {/each}
  </div>

  <div class="mt-1 grid grid-cols-7 gap-1" data-testid="mentor-own-calendar">
    {#each weeks as week, w (w)}
      {#each week as day, d (day || `blank-${w}-${d}`)}
        {#if day === ''}
          <span></span>
        {:else}
          <button
            type="button"
            aria-pressed={selectedDay === day}
            class="flex aspect-square flex-col items-center justify-center gap-1 rounded text-sm
                   {selectedDay === day ? 'bg-primary text-primary-foreground' : 'hover:bg-muted'}"
            onclick={() => (selectedDay = day)}
          >
            <span>{Number(day.slice(8, 10))}</span>
            <span class="flex gap-0.5">
              {#each dayStatuses(byDay.get(day) ?? []) as status (status)}
                <span class="h-1 w-1 rounded-full {STATUS_DOT[status]}"></span>
              {/each}
            </span>
          </button>
        {/if}
      {/each}
    {/each}
  </div>

  {#if loading}
    <Skeleton class="mt-3 h-4 w-32" />
  {/if}
  {#if failed}
    <p class="text-muted-foreground mt-3 text-sm">
      We couldn't load your calendar just now. Try picking the month again.
    </p>
  {/if}

  {#if selectedDay}
    <div class="mt-4 border-t pt-4">
      <p class="mb-2 text-sm font-medium">{selectedDay}</p>
      {#if dayIntervals.length === 0}
        <p class="text-muted-foreground text-sm">No data for this day.</p>
      {:else}
        <ul class="flex flex-col gap-1">
          {#each dayIntervals as iv (iv.starts_at + iv.status)}
            <li class="flex items-center justify-between text-sm">
              <span>{fmtInstant(iv.starts_at)} – {fmtInstant(iv.ends_at)}</span>
              <Badge variant="secondary">{STATUS_LABEL[iv.status]}</Badge>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  <p class="text-muted-foreground mt-4 text-xs">
    Times shown in {calendar.timezone}. To change your availability, edit it below.
  </p>
</Card>
