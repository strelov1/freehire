<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import {
    buildActivityGrid,
    LEVELS,
    rangeForWindow,
    type ActivityDay,
    type ActivityWeek,
  } from '$lib/activityGrid';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { plural, t } from '$lib/i18n/t';
  import type { TimelineEvent } from '$lib/types';
  import { Button, Card } from '$lib/ui';
  import { messages } from './ActivityGrid.messages';
  import ApplicationEventList from './ApplicationEventList.svelte';
  import States from './States.svelte';

  // A year of the caller's own job-search actions, drawn the way a contribution graph is.
  //
  // The server load hands over the window fetched in ITS timezone; which square an event
  // lands on is decided here, because only the browser knows the reader's clock. See
  // activityGrid.ts — the arithmetic, the counting rule and the streaks all live there,
  // and this component renders the model and nothing else.
  let { prefetched }: { prefetched: TimelineEvent[] | undefined } = $props();

  const s = $derived(t(messages, locale()));

  // The prop is a fallback, not a seed: copying it into state freezes it at its first value.
  // Once the client has fetched a window of its own, that is what the grid reads.
  let fetched = $state<TimelineEvent[] | null>(null);
  const series = $derived(fetched ?? prefetched ?? []);
  let phase = $state<'initial' | 'loading' | 'ready' | 'error'>('initial');
  // 'initial' means nothing has been asked for yet: usable when the server load answered,
  // waiting on the mount fetch otherwise. Reading the prop here rather than in an initialiser
  // is what keeps it live instead of frozen at its first value.
  const status = $derived.by(() => {
    if (phase !== 'initial') return phase;
    return prefetched ? 'ready' : 'loading';
  });

  let selectedKey = $state<string | null>(null);

  // One fetch on mount, and only when the server load did not answer. Not reactive: nothing
  // moves the window, so nothing re-fetches it.
  onMount(() => {
    if (!prefetched) void load();
  });

  async function load() {
    phase = 'loading';
    try {
      const { from, to } = rangeForWindow();
      fetched = await api.myTimeline(from, to);
      phase = 'ready';
    } catch {
      phase = 'error';
    }
  }

  const grid = $derived(buildActivityGrid(series));
  const selected = $derived(grid.days.find((d) => d.key === selectedKey) ?? null);

  // Rows are weekdays and columns are weeks, so the rail runs down the side rather than along
  // the top as the calendar's does. Only alternate rows are labelled — seven captions beside
  // 12px squares is noise.
  //
  // Read off the first column's own dates rather than written out, so they are in the reader's
  // language and start on the day the model actually starts on. A hardcoded 'Mon' would be
  // English on a Russian page AND would silently lie if the model ever moved to Sunday-first.
  const LABELLED_ROWS = new Set([0, 2, 4]);
  const weekdayLabels = $derived(
    (grid.weeks[0]?.days ?? []).map((day, row) =>
      LABELLED_ROWS.has(row) ? day.date.toLocaleDateString(locale(), { weekday: 'short' }) : '',
    ),
  );

  /** A column's caption. WHICH column carries one is the model's decision
   *  (`ActivityWeek.monthStart` is set only where a month is introduced); what it SAYS is
   *  this component's, because the model does not know the reader's locale. */
  const monthLabel = (week: ActivityWeek) =>
    week.monthStart ? week.monthStart.toLocaleDateString(locale(), { month: 'short' }) : '';

  /** The shade for a level. Tokens with an alpha modifier rather than palette utilities: the
   *  theme must be able to move this scale, and `bg-emerald-500` is a fixed hue `.dark` gets
   *  no say in. */
  const SHADES = ['bg-muted', 'bg-primary/25', 'bg-primary/45', 'bg-primary/70', 'bg-primary'];

  const dayHeading = (d: ActivityDay) =>
    d.date.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });

  const countOf = (n: number) => `${n} ${plural(locale(), n, s.actions)}`;
  const daysOf = (n: number) => `${n} ${plural(locale(), n, s.days)}`;


  /** What a square says on hover and to a screen reader. A day with nothing on it says so in
   *  words rather than "0 actions", which across three hundred empty squares is noise a screen
   *  reader has to walk through one at a time. */
  const cellLabel = (d: ActivityDay) =>
    `${d.count > 0 ? countOf(d.count) : s.noActions} — ${dayHeading(d)}`;

  // A year of squares is tall enough that the panel can open below the fold, where the click
  // reads as having done nothing. Only scrolls when it actually is out of view.
  async function selectDay(key: string) {
    selectedKey = selectedKey === key ? null : key;
    if (!selectedKey) return;
    await tick();
    const panel = document.getElementById('activity-day-panel');
    if (panel && panel.getBoundingClientRect().bottom > window.innerHeight) {
      panel.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }

  // The grid is wider than a phone, so it scrolls — and it opens showing TODAY rather than a
  // year ago, which is the end everybody came to look at.
  let scroller = $state<HTMLDivElement | null>(null);
  $effect(() => {
    // Reading `grid` is what makes this run once the model exists.
    if (scroller && grid.weeks.length > 0) scroller.scrollLeft = scroller.scrollWidth;
  });
</script>

<div class="flex flex-col gap-4">
  {#if status === 'error'}
    <States state="error" message={s.loadError} />
  {:else if status === 'loading'}
    <States state="loading" rows={3} />
  {:else}
    <Card class="p-4">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="text-lg font-medium">{s.heading}</h2>
        <p class="text-sm text-muted-foreground">{countOf(grid.total)}</p>
      </div>
      <p class="mt-1 text-sm text-muted-foreground">{s.caption}</p>

      <!-- `activity-scroller` hides the scrollbar without taking the scrolling away: the grid
           is a year wide, it opens at today, and an overlay bar appearing over the squares on
           hover reads as part of the chart. Keyboard and wheel scrolling are untouched.
           A plain class rather than a utility because Tailwind ships none for this. -->
      <div bind:this={scroller} class="activity-scroller mt-4 overflow-x-auto pb-1">
        <!-- pr-1 is not cosmetic: today's square carries a `ring`, which paints OUTSIDE its
             box, and the view opens scrolled hard to the right — so without a gutter the ring
             is clipped by the card edge on the one square a reader came to look at. -->
        <div class="flex w-max gap-1 pr-1">
          <!-- The weekday rail. aria-hidden: every square already announces its own full
               date, so these three labels would only repeat it. -->
          <div class="flex flex-col gap-0.5 pt-4 pr-1" aria-hidden="true">
            {#each weekdayLabels as label, row (row)}
              <div class="flex h-3 items-center text-xs leading-none text-muted-foreground">{label}</div>
            {/each}
          </div>

          {#each grid.weeks as week (week.key)}
            <div class="flex flex-col gap-0.5">
              <div class="h-4 text-xs leading-none text-muted-foreground" aria-hidden="true">
                {monthLabel(week)}
              </div>
              {#each week.days as day (day.key)}
                {#if day.inWindow}
                  <!-- rounded-none, not the radius token, for the reason TrackingCalendar
                       states about its own marks: radius-sm is 6px, and 6px on a 12px box is a
                       circle. These shipped as dots until somebody looked at the screen. -->
                  <button
                    type="button"
                    onclick={() => selectDay(day.key)}
                    aria-expanded={selectedKey === day.key}
                    aria-controls={selectedKey === day.key ? 'activity-day-panel' : undefined}
                    aria-label={cellLabel(day)}
                    title={cellLabel(day)}
                    class="size-3 rounded-none border border-transparent transition-colors hover:border-foreground
                           {SHADES[day.level]}
                           {day.isToday ? 'ring-1 ring-foreground' : ''}
                           {selectedKey === day.key ? 'border-foreground' : ''}"
                  ></button>
                {:else}
                  <!-- A cell outside the window: it exists to keep the row seven long, and is
                       NOT a day we measured. Drawing it as a level-zero square would claim
                       we looked. -->
                  <div class="size-3" aria-hidden="true"></div>
                {/if}
              {/each}
            </div>
          {/each}
        </div>
      </div>

      <div class="mt-3 flex items-center justify-end gap-1 text-xs text-muted-foreground">
        <span>{s.legendLess}</span>
        {#each Array(LEVELS + 1) as _, level (level)}
          <span class="size-3 rounded-none {SHADES[level]}" aria-hidden="true"></span>
        {/each}
        <span>{s.legendMore}</span>
      </div>
    </Card>

    <div class="grid gap-3 sm:grid-cols-3">
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">{s.totalLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{grid.total}</p>
      </Card>
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">{s.currentStreakLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{daysOf(grid.currentStreak)}</p>
      </Card>
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">{s.longestStreakLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{daysOf(grid.longestStreak)}</p>
      </Card>
    </div>

    {#if selected}
      <!-- Assembled from the events already fetched for the window. Selecting a day issues no
           request — and it lists EVERY event of that day, including the ones that do not shade
           the square, because what a square measures is effort and what a day held is history. -->
      <Card class="p-4">
        <div id="activity-day-panel" role="region" aria-live="polite">
          <h3 class="mb-3 text-sm font-medium">
            {dayHeading(selected)}
            {#if selected.count > 0}
              <span class="font-normal text-muted-foreground">· {countOf(selected.count)}</span>
            {/if}
          </h3>
          {#if selected.events.length === 0}
            <p class="text-sm text-muted-foreground">{s.panelNothing}</p>
          {:else}
            <ApplicationEventList events={selected.events} />
          {/if}
        </div>
      </Card>
    {/if}

    {#if grid.total === 0}
      <Card class="p-4">
        <p class="text-sm text-muted-foreground">{s.empty}</p>
        <Button variant="outline" size="sm" class="mt-3" href={resolve('/my/tracking')}>{s.emptyCta}</Button>
      </Card>
    {/if}
  {/if}
</div>

<style>
  /* Scrolling without the scrollbar. The grid is a year wide and opens at today, so the bar
     has nothing to tell a reader they do not already know — while an overlay one, which is
     what macOS draws on hover, appears ON TOP of the squares and reads as part of the chart.
     Both vendor spellings, because `scrollbar-width` is unsupported in Safari and
     `::-webkit-scrollbar` in Firefox; one alone leaves the bar on half the browsers.
     Deliberately NOT `overflow: hidden` — the wheel, the trackpad, the keyboard and the
     scroll-to-today effect all still have to work. */
  .activity-scroller {
    scrollbar-width: none;
  }
  .activity-scroller::-webkit-scrollbar {
    display: none;
  }
</style>
