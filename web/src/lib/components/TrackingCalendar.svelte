<script lang="ts">
  import { ChevronLeft, ChevronRight } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { eventTone } from '#lib/events.js';
  import {
    buildCalendarWeek,
    calendarDayKey,
    monthLabel,
    rangeForWeek,
    splitDayEvents,
    type CalendarDay,
  } from '#lib/calendarModel.js';
  import { boardRefFor } from '#lib/board.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import type { ScheduledInterview, TimelineEvent } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import ApplicationEventList from './ApplicationEventList.svelte';
  import States from './States.svelte';
  import { messages } from './TrackingCalendar.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';

  // The server load hands over one month, fetched in ITS timezone; everything after that
  // is fetched here, because only the browser knows the reader's. See calendarModel.
  let {
    prefetched,
  }: {
    prefetched:
      | { events: TimelineEvent[]; interviews: ScheduledInterview[]; year: number; month: number }
      | undefined;
  } = $props();

  const s = $derived(t(messages, locale()));

  const now = new Date();
  let cursor = $state(new Date(now.getFullYear(), now.getMonth(), now.getDate()));
  const year = $derived(cursor.getFullYear());
  const month = $derived(cursor.getMonth());
  let selectedKey = $state(calendarDayKey(now));
  // The server payload is a fallback, not a seed: copying a prop into state freezes it at
  // its first value, and the two would then disagree after a navigation. Once the client
  // has fetched a month of its own, that is what the grid reads.
  let fetched = $state<{ events: TimelineEvent[]; interviews: ScheduledInterview[] } | null>(null);
  const series = $derived(fetched?.events ?? prefetched?.events ?? []);
  const meetings = $derived(fetched?.interviews ?? prefetched?.interviews ?? []);
  // 'initial' means nothing has been asked for yet: usable if the server load succeeded
  // for the month on screen, waiting on the mount fetch otherwise. Reading the prop here
  // rather than in an initialiser is what keeps it live instead of frozen at first value.
  let phase = $state<'initial' | 'loading' | 'ready' | 'error'>('initial');
  const status = $derived.by(() => (phase === 'initial' ? (serverMonthIsOurs ? 'ready' : 'loading') : phase));

  // The server's month is not necessarily the reader's — a render at 18:00 in Los Angeles
  // on 31 July happens on 1 August in a UTC process. The margin rangeForMonth adds is
  // sized for a day boundary and cannot absorb a month boundary, so when the two disagree
  // the payload is for a month we are not drawing and has to be replaced.
  const serverMonthIsOurs = $derived(prefetched?.year === year && prefetched?.month === month);

  // Only the newest request may write. Stepping twice quickly leaves two in flight, and
  // without this the slower one lands last and paints the wrong week's events.
  let inFlight = 0;

  // Navigation moves the cursor and fetches its week; choosing a day reuses that data.
  async function show(date: Date) {
    cursor = date;
    selectedKey = calendarDayKey(date);
    phase = 'loading';
    const generation = ++inFlight;
    const { from, to } = rangeForWeek(date);
    try {
      // Both layers together, so selecting a day still needs no request of its own.
      const [events, interviews] = await Promise.all([api.myTimeline(from, to), api.myInterviews(from, to)]);
      if (generation !== inFlight) return;
      fetched = { events, interviews };
      phase = 'ready';
    } catch {
      if (generation !== inFlight) return;
      phase = 'error';
    }
  }

  // Whether this reader has given us their calendar, and whether the flow exists to give
  // it. Undefined until asked, so the invitation below does not flash for someone who
  // already connected one — or on a deployment that has no Google client configured,
  // where the connect route is not registered at all and the button would navigate the
  // browser to a JSON 404. The Gmail surface states that invariant for its own button;
  // `available` is what carries it, and it is served already.
  let calendarConnected = $state<boolean | undefined>(undefined);
  let connectAvailable = $state(false);

  // One fetch on mount when the server load failed, or when it answered for a month other
  // than the reader's. Once, not reactively.
  onMount(() => {
    if (!serverMonthIsOurs) void show(cursor);
    if (isAuthenticated()) {
      void api
        .gmailStatus()
        .then((gmailStatus) => {
          calendarConnected = gmailStatus.calendar_connected === true;
          connectAvailable = gmailStatus.available === true;
        })
        .catch(() => (calendarConnected = undefined));
    }
  });

  const days = $derived(buildCalendarWeek(cursor, series, meetings));
  const selected = $derived(days.find((d) => d.key === selectedKey) ?? null);
  const weekdays = $derived(s.weekdays);
  const CELL_MARKS = 3;

  function selectDay(key: string) {
    selectedKey = key;
  }

  function step(by: number) {
    const date = selected?.date ?? cursor;
    void show(new Date(date.getFullYear(), date.getMonth(), date.getDate() + by * 7));
  }

  const weekRange = $derived(days.length
    ? `${days[0]?.date.toLocaleDateString(locale(), { month: 'short', day: 'numeric' })} – ${days[6]?.date.toLocaleDateString(locale(), { month: 'short', day: 'numeric' })}`
    : '');

  // The mark colours carry the kind; filled versus hollow carries whether anybody but the
  // candidate set the date. Both the label and the tone come from #lib/events, shared with
  // the application panel:
  // the same event captioned two ways on two screens is what putting them here caused.
  const tone = (kind: string) => eventTone(kind);

  // Only the meetings are clocked here now; a ledger event's own time is rendered by
  // ApplicationEventList, which both day panels share.
  const clockOf = (instant: string) =>
    new Date(instant).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' });

  /** An arranged meeting is drawn as arranged, never as observed: it has not happened
   *  yet and may still move. A cancelled one keeps its place — an interview that simply
   *  vanished from a Thursday cannot be told apart from a calendar that failed to load. */
  const meetingTone = (iv: ScheduledInterview) =>
    iv.status === 'cancelled' ? 'text-muted-foreground' : 'text-brand-strong';

  const dayHeading = (d: CalendarDay) =>
    d.date.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long' });

  /** What a cell announces. An empty day says nothing about a count: "0 events" on the
   *  thirty-odd empty cells is noise a screen reader has to walk through. */
  /** "1 entry" / "3 entries". Shared by the cell's label and the panel's heading so the
   *  two cannot disagree, which is how "1 entries" got onto the screen in the first place. */
  function entryCount(d: CalendarDay): string {
    const n = d.events.length + d.interviews.length;
    return format(plural(locale(), n, s.entries), { count: String(n) });
  }

  function cellLabel(d: CalendarDay): string {
    if (d.events.length + d.interviews.length === 0) return dayHeading(d);
    const scheduled = d.interviews.length > 0 ? s.interviewScheduledSuffix : '';
    return `${dayHeading(d)} — ${entryCount(d)}${scheduled}`;
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex items-center justify-between gap-3">
    <div>
      <h2 class="text-lg font-medium">{monthLabel(year, month)}</h2>
      <p class="text-xs text-muted-foreground">{weekRange}</p>
    </div>
    <div class="flex items-center gap-1">
      <Button variant="ghost" size="sm" onclick={() => show(new Date())}>{s.today}</Button>
      <Button variant="outline" size="icon" onclick={() => step(-1)} aria-label={s.previousWeek}>
        <ChevronLeft class="size-4" />
      </Button>
      <Button variant="outline" size="icon" onclick={() => step(1)} aria-label={s.nextWeek}>
        <ChevronRight class="size-4" />
      </Button>
    </div>
  </div>

  {#if status === 'error'}
    <States state="error" message={s.loadError} />
  {:else if status === 'loading'}
    <States state="loading" rows={3} />
  {:else}
    <div class="grid grid-cols-7 gap-1 border-b border-border pb-3" role="group" aria-label={s.weekLabel}>
      {#each days as day, index (day.key)}
        {@const marks = splitDayEvents(day.events, CELL_MARKS)}
        <button type="button" onclick={() => selectDay(day.key)} aria-pressed={selectedKey === day.key}
          aria-controls="calendar-day-panel" aria-label={cellLabel(day)}
          class="flex min-h-20 min-w-0 flex-col items-center gap-1.5 rounded-lg py-2 text-center transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring">
          <span class="text-xs text-muted-foreground">{weekdays[index]}</span>
          <span class="flex size-8 items-center justify-center rounded-full text-sm font-medium
            {selectedKey === day.key ? 'bg-brand text-brand-foreground' : day.isToday ? 'bg-brand-muted text-brand-strong' : 'text-foreground'}">{day.dayOfMonth}</span>
          <span class="flex h-2 items-center justify-center gap-0.5" aria-hidden="true">
            {#if day.interviews.length}<span class="size-1.5 bg-brand" title={s.interview}></span>{/if}
            {#each marks.shown as e (e.id)}<span class="size-1.5 rounded-full {tone(e.kind)}" class:bg-current={e.observed} style="border: 1px solid currentColor"></span>{/each}
            {#if marks.remaining}<span class="text-xs text-muted-foreground">+</span>{/if}
          </span>
        </button>
      {/each}
    </div>

    {#if selected}
      <!-- Assembled from the events already fetched for the range. Selecting a day issues
           no request, which is also what keeps this panel from ever reaching the message
           endpoint — that one marks mail read. -->
      <div id="calendar-day-panel" role="region" aria-live="polite" class="rounded-lg border bg-card p-4">
        <h3 class="mb-3 text-sm font-medium">
          {dayHeading(selected)}
          <span class="font-normal text-muted-foreground">· {entryCount(selected)}</span>
        </h3>
        {#if selected.interviews.length > 0}
          <ul class="mb-3 flex flex-col gap-3">
            {#each selected.interviews as iv (iv.id)}
              <li class="flex gap-3">
                <span
                  class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-none border {meetingTone(iv)}"
                  class:bg-current={iv.status !== 'cancelled'}
                  style="border-color: currentColor"
                ></span>
                <div class="min-w-0 flex-1">
                  <p class="text-sm" class:line-through={iv.status === 'cancelled'}>
                    <span class="font-medium">{iv.company_slug}</span>
                    {#if iv.role_title}<span class="text-muted-foreground"> · {iv.role_title}</span>{/if}
                  </p>
                  <p class="text-sm text-muted-foreground">
                    {iv.title || s.interview}
                    {#if iv.status === 'cancelled'}<span class="font-medium">{s.cancelledSuffix}</span>{/if}
                    {#if iv.status === 'suggested'}<span>{s.unconfirmedSuffix}</span>{/if}
                  </p>
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {clockOf(iv.starts_at)}
                    {#if iv.application_id}
                      · <a
                          class="underline hover:no-underline"
                          href={resolve('/my/tracking/[id]', { id: boardRefFor(iv) ?? '' })}>{s.applicationLink}</a
                        >
                    {/if}
                    {#if iv.join_url && iv.status !== 'cancelled'}
                      ·
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the organiser's meeting link, an external URL rather than an internal route -->
                      <a class="underline hover:no-underline" href={iv.join_url} rel="noreferrer">{s.joinLink}</a>
                    {/if}
                  </p>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
        {#if selected.events.length === 0 && selected.interviews.length === 0}
          <p class="text-sm text-muted-foreground">{s.nothingThisDay}</p>
        {:else if selected.events.length > 0}
          <ApplicationEventList events={selected.events} />
        {/if}
      </div>
    {/if}

    {#if calendarConnected === false && connectAvailable}
      <!-- Said plainly rather than discovered at Google's consent screen. The OAuth app
           is not verified yet, so an account outside the test roster is simply refused
           there — and a refusal with no explanation reads as a fault in freehire. The
           connect action itself lives on Integrations, alongside Mail and every other
           third-party connection, not inline here. -->
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm">
          {s.connectBanner.lead}
        </p>
        <p class="mt-1 text-sm text-muted-foreground">
          {s.connectBanner.detail}
        </p>
        <Button variant="outline" size="sm" class="mt-3" href={resolve('/my/integrations')}>
          {s.connectBanner.cta}
        </Button>
      </div>
    {/if}

  {/if}
</div>
