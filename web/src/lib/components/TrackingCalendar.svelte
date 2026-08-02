<script lang="ts">
  import { ChevronLeft, ChevronRight } from '@lucide/svelte';
  import { onMount, tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import {
    buildCalendarMonth,
    monthLabel,
    rangeForMonth,
    splitDayEvents,
    type CalendarDay,
  } from '$lib/calendarModel';
  import { boardRefFor } from '$lib/board';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { ScheduledInterview, TimelineEvent } from '$lib/types';
  import { Button } from '$lib/ui';
  import States from './States.svelte';

  // The server load hands over one month, fetched in ITS timezone; everything after that
  // is fetched here, because only the browser knows the reader's. See calendarModel.
  let {
    prefetched,
  }: {
    prefetched:
      | { events: TimelineEvent[]; interviews: ScheduledInterview[]; year: number; month: number }
      | undefined;
  } = $props();

  const now = new Date();
  let year = $state(now.getFullYear());
  let month = $state(now.getMonth());
  let selectedKey = $state<string | null>(null);
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
  // without this the slower one lands last and paints one month's events under another's
  // grid — with phase 'ready' and nothing to say the data is not this month's.
  let inFlight = 0;

  // Deliberately not an $effect: this function writes year and month, and an effect that
  // read them would re-run on its own writes. Navigation is the only thing that moves the
  // cursor, so navigation fetches.
  async function show(y: number, m: number) {
    year = y;
    month = m;
    selectedKey = null;
    phase = 'loading';
    const generation = ++inFlight;
    const { from, to } = rangeForMonth(y, m);
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
    if (!serverMonthIsOurs) void show(year, month);
    if (isAuthenticated()) {
      void api
        .gmailStatus()
        .then((s) => {
          calendarConnected = s.calendar_connected === true;
          connectAvailable = s.available === true;
        })
        .catch(() => (calendarConnected = undefined));
    }
  });

  const grid = $derived(buildCalendarMonth(year, month, series, meetings));
  const selected = $derived(grid.days.find((d) => d.key === selectedKey) ?? null);
  const weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
  const CELL_MARKS = 4;

  // A month of mostly-empty cells is tall, so on a laptop the panel opens below the fold
  // and the click reads as having done nothing. Only scrolls when it actually is out of
  // view, and only as far as it must — an unconditional jump is worse than the problem.
  async function selectDay(key: string) {
    selectedKey = selectedKey === key ? null : key;
    if (!selectedKey) return;
    await tick();
    const panel = document.getElementById('calendar-day-panel');
    if (panel && panel.getBoundingClientRect().bottom > window.innerHeight) {
      panel.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }

  function step(by: number) {
    const d = new Date(year, month + by, 1);
    void show(d.getFullYear(), d.getMonth());
  }

  // The mark colours carry the kind; filled versus hollow carries whether anybody but the
  // candidate set the date. An unrecognised kind gets the neutral mark rather than none —
  // a fifth kind is coming and must be visible before this file knows its name.
  //
  // Tokens, not raw palette hues. Four arbitrary colours would read as a fifth vocabulary
  // nobody owns, and `pnpm check:tokens` counts them per file; the neighbouring status maps
  // predate the check and carry a recorded baseline, which is not a reason to add to it.
  //
  // Only four token colours are actually distinct: primary and secondary-foreground hold
  // the SAME value in both themes (oklch(0.13 0 0) light, oklch(0.985 0 0) dark), so
  // reaching for the second to separate two kinds silently collapses them into one mark.
  const KIND_TONE: Record<string, string> = {
    applied: 'text-primary',
    employer_reply: 'text-brand-strong',
    follow_up_sent: 'text-warning-strong',
    stage_set: 'text-muted-foreground',
  };
  const tone = (kind: string) => KIND_TONE[kind] ?? 'text-muted-foreground';

  /** Sentence-case an unknown kind so a new one reads as words, not as a column name. */
  function humanKind(kind: string): string {
    const words = kind.replace(/_/g, ' ');
    return words.charAt(0).toUpperCase() + words.slice(1);
  }

  const KIND_LABEL: Record<string, (e: TimelineEvent) => string> = {
    applied: () => 'Applied',
    employer_reply: (e) => (e.signal ? `Employer replied — ${e.signal.replace(/_/g, ' ')}` : 'Employer replied'),
    follow_up_sent: () => 'Followed up',
    stage_set: (e) => (e.signal ? `Moved to ${e.signal}` : 'Stage changed'),
  };
  const label = (e: TimelineEvent) => (KIND_LABEL[e.kind] ?? (() => humanKind(e.kind)))(e);

  const timeOf = (e: TimelineEvent) => clockOf(e.occurred_at);
  const clockOf = (instant: string) =>
    new Date(instant).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });

  /** An arranged meeting is drawn as arranged, never as observed: it has not happened
   *  yet and may still move. A cancelled one keeps its place — an interview that simply
   *  vanished from a Thursday cannot be told apart from a calendar that failed to load. */
  const meetingTone = (iv: ScheduledInterview) =>
    iv.status === 'cancelled' ? 'text-muted-foreground' : 'text-brand-strong';

  const dayHeading = (d: CalendarDay) =>
    d.date.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' });

  /** What a cell announces. An empty day says nothing about a count: "0 events" on the
   *  thirty-odd empty cells is noise a screen reader has to walk through. */
  /** "1 entry" / "3 entries". Shared by the cell's label and the panel's heading so the
   *  two cannot disagree, which is how "1 entries" got onto the screen in the first place. */
  function entryCount(d: CalendarDay): string {
    const n = d.events.length + d.interviews.length;
    return `${n} ${n === 1 ? 'entry' : 'entries'}`;
  }

  function cellLabel(d: CalendarDay): string {
    if (d.events.length + d.interviews.length === 0) return dayHeading(d);
    const scheduled = d.interviews.length > 0 ? ', interview scheduled' : '';
    return `${dayHeading(d)} — ${entryCount(d)}${scheduled}`;
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex items-center justify-between gap-3">
    <h2 class="text-lg font-medium">{monthLabel(year, month)}</h2>
    <div class="flex items-center gap-1">
      <Button variant="outline" size="icon" onclick={() => step(-1)} aria-label="Previous month">
        <ChevronLeft class="size-4" />
      </Button>
      <Button variant="outline" size="icon" onclick={() => step(1)} aria-label="Next month">
        <ChevronRight class="size-4" />
      </Button>
    </div>
  </div>

  {#if status === 'error'}
    <States state="error" message="Couldn't load what happened this month." />
  {:else if status === 'loading'}
    <States state="loading" rows={3} />
  {:else}
    <!-- The grid below seven columns is unreadable on a phone, so the same month is
         presented there as the list of days that hold something. -->
    <div class="hidden rounded-lg border bg-card p-3 sm:block">
      <div class="mb-1 grid grid-cols-7 gap-1">
        {#each weekdays as w (w)}
          <div class="px-1 py-0.5 text-xs font-medium text-muted-foreground">{w}</div>
        {/each}
      </div>
      <div class="grid grid-cols-7 gap-1">
        {#each grid.days as day (day.key)}
          {@const marks = splitDayEvents(day.events, CELL_MARKS)}
          <button
            type="button"
            onclick={() => selectDay(day.key)}
            aria-expanded={selectedKey === day.key}
            aria-controls={selectedKey === day.key ? 'calendar-day-panel' : undefined}
            aria-label={cellLabel(day)}
            class="flex min-h-16 flex-col items-start gap-1 rounded-md border p-1.5 text-left transition-colors
                   {day.inMonth ? '' : 'opacity-45'}
                   {selectedKey === day.key ? 'border-primary bg-accent' : 'hover:bg-accent'}"
          >
            <span
              class="text-xs {day.isToday
                ? 'flex h-5 w-5 items-center justify-center rounded-full bg-primary font-medium text-primary-foreground'
                : 'text-muted-foreground'}">{day.dayOfMonth}</span
            >
            <span class="flex flex-wrap items-center gap-0.5">
              {#each day.interviews as iv (iv.id)}
                <!-- A square marks an appointment, a circle a record. Shape rather than
                     hue alone, so the difference does not depend on seeing colour.
                     rounded-none and not the radius token: radius-sm is 6px, which on an
                     8px mark rounds the square back into a circle — the distinction was
                     invisible on screen until it was looked at. -->
                <span
                  class="inline-block h-2 w-2 rounded-none border {meetingTone(iv)}"
                  class:bg-current={iv.status !== 'cancelled'}
                  style="border-color: currentColor"
                  title={iv.status === 'cancelled' ? 'Interview — cancelled' : 'Interview'}
                ></span>
              {/each}
              {#each marks.shown as e (e.id)}
                <!-- Filled = a date somebody other than the candidate set. Hollow = one
                     they recorded themselves. -->
                <span
                  class="inline-block h-2 w-2 rounded-full border {tone(e.kind)}"
                  class:bg-current={e.observed}
                  style="border-color: currentColor"
                  title={label(e)}
                ></span>
              {/each}
              {#if marks.remaining > 0}
                <span class="text-xs leading-none text-muted-foreground">+{marks.remaining}</span>
              {/if}
            </span>
          </button>
        {/each}
      </div>
    </div>

    <div class="flex flex-col gap-2 sm:hidden">
      {#each grid.daysWithEvents as day (day.key)}
        <button
          type="button"
          onclick={() => selectDay(day.key)}
          aria-pressed={selectedKey === day.key}
          class="flex items-center justify-between gap-3 rounded-lg border bg-card p-3 text-left text-sm
                 {selectedKey === day.key ? 'border-primary' : ''}"
        >
          <span>{dayHeading(day)}</span>
          <span class="text-muted-foreground">{day.events.length}</span>
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
                    {iv.title || 'Interview'}
                    {#if iv.status === 'cancelled'}<span class="font-medium"> — cancelled</span>{/if}
                    {#if iv.status === 'suggested'}<span> — unconfirmed</span>{/if}
                  </p>
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {clockOf(iv.starts_at)}
                    {#if iv.application_id}
                      · <a
                          class="underline hover:no-underline"
                          href={resolve('/my/tracking/[id]', { id: boardRefFor(iv) ?? '' })}>application</a
                        >
                    {/if}
                    {#if iv.join_url && iv.status !== 'cancelled'}
                      ·
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the organiser's meeting link, an external URL rather than an internal route -->
                      <a class="underline hover:no-underline" href={iv.join_url} rel="noreferrer">join</a>
                    {/if}
                  </p>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
        {#if selected.events.length === 0 && selected.interviews.length === 0}
          <p class="text-sm text-muted-foreground">Nothing happened on this day.</p>
        {:else if selected.events.length > 0}
          <ul class="flex flex-col gap-3">
            {#each selected.events as e (e.id)}
              <li class="flex gap-3">
                <span
                  class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-full border {tone(e.kind)}"
                  class:bg-current={e.observed}
                  style="border-color: currentColor"
                ></span>
                <div class="min-w-0 flex-1">
                  <p class="text-sm">
                    <span class="font-medium">{e.company_slug}</span>
                    {#if e.role_title}<span class="text-muted-foreground"> · {e.role_title}</span>{/if}
                  </p>
                  <p class="text-sm text-muted-foreground">{label(e)}</p>
                  {#if e.email_subject}
                    <p class="truncate text-sm italic text-muted-foreground">“{e.email_subject}”</p>
                  {/if}
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {#if e.observed}
                      {timeOf(e)}
                    {:else}
                      recorded by you
                    {/if}
                    {#if boardRefFor(e)}
                      · <a
                          class="underline hover:no-underline"
                          href={resolve('/my/tracking/[id]', { id: boardRefFor(e) ?? '' })}>application</a
                        >
                    {/if}
                    {#if e.email_id}
                      ·
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve()d base plus a query string; there is no dynamic route segment to resolve -->
                      <a class="underline hover:no-underline" href={`${resolve('/my/inbox')}?message=${e.email_id}`}
                        >message</a
                      >
                    {/if}
                  </p>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}

    {#if calendarConnected === false && connectAvailable}
      <!-- Said plainly rather than discovered at Google's consent screen. The OAuth app
           is not verified yet, so an account outside the test roster is simply refused
           there — and a refusal with no explanation reads as a fault in freehire. -->
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm">
          Connect your calendar and the interviews you accept will appear here, on the day they
          are due.
        </p>
        <p class="mt-1 text-sm text-muted-foreground">
          Only meetings we can attach to one of your applications are stored — the rest of your
          calendar is read and discarded. While our Google app is awaiting verification the
          connection works for approved test accounts only.
        </p>
        <!-- eslint-disable svelte/no-navigation-without-resolve -- an API route the browser must
             navigate to so Google can redirect it back, not a SvelteKit page to resolve. The
             block form because prettier puts href on its own line, past a next-line disable. -->
        <a
          href="/api/v1/me/calendar/connect"
          class="mt-3 inline-block rounded-md border px-3 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground"
          >Connect Google Calendar</a
        >
        <!-- eslint-enable svelte/no-navigation-without-resolve -->
      </div>
    {/if}

    {#if grid.total === 0}
      <p class="text-sm text-muted-foreground">
        Nothing recorded in {monthLabel(year, month)}. Applications you track, and the replies they
        get, appear here — start from the
        <a class="underline hover:no-underline" href={resolve('/my/tracking')}>board</a>.
      </p>
    {/if}
  {/if}
</div>
