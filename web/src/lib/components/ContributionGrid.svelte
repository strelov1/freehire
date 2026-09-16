<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { boardRefFor } from '$lib/board';
  import {
    buildActivityGrid,
    LEVELS,
    rangeForWindow,
    type ActivityDay,
    type ActivityWeek,
  } from '$lib/contributionGrid';
  import { eventLabel, eventTone } from '$lib/events';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { plural, t } from '$lib/i18n/t';
  import type { TimelineEvent } from '$lib/types';
  import { Button } from '$lib/ui';
  import { messages } from './ContributionGrid.messages';
  import States from './States.svelte';

  // A year of the caller's own job-search actions, drawn the way a contribution graph is.
  //
  // The server load hands over the window fetched in ITS timezone; which square an event
  // lands on is decided here, because only the browser knows the reader's clock. See
  // contributionGrid.ts — the arithmetic, the counting rule and the streaks all live there,
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

  const timeOf = (instant: string) =>
    new Date(instant).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' });

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
    <div class="rounded-lg border bg-card p-4">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="text-lg font-medium">{s.heading}</h2>
        <p class="text-sm text-muted-foreground">{countOf(grid.total)}</p>
      </div>
      <p class="mt-1 text-sm text-muted-foreground">{s.caption}</p>

      <div bind:this={scroller} class="mt-4 overflow-x-auto pb-1">
        <div class="flex w-max gap-1">
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
                  <button
                    type="button"
                    onclick={() => selectDay(day.key)}
                    aria-expanded={selectedKey === day.key}
                    aria-controls={selectedKey === day.key ? 'activity-day-panel' : undefined}
                    aria-label={cellLabel(day)}
                    title={cellLabel(day)}
                    class="size-3 rounded-sm border border-transparent transition-colors hover:border-foreground
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
          <span class="size-3 rounded-sm {SHADES[level]}" aria-hidden="true"></span>
        {/each}
        <span>{s.legendMore}</span>
      </div>
    </div>

    <div class="grid gap-3 sm:grid-cols-3">
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm text-muted-foreground">{s.totalLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{grid.total}</p>
      </div>
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm text-muted-foreground">{s.currentStreakLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{daysOf(grid.currentStreak)}</p>
      </div>
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm text-muted-foreground">{s.longestStreakLabel}</p>
        <p class="text-2xl font-semibold tabular-nums">{daysOf(grid.longestStreak)}</p>
      </div>
    </div>

    {#if selected}
      <!-- Assembled from the events already fetched for the window. Selecting a day issues no
           request — and it lists EVERY event of that day, including the ones that do not shade
           the square, because what a square measures is effort and what a day held is history. -->
      <div id="activity-day-panel" role="region" aria-live="polite" class="rounded-lg border bg-card p-4">
        <h3 class="mb-3 text-sm font-medium">
          {dayHeading(selected)}
          {#if selected.count > 0}
            <span class="font-normal text-muted-foreground">· {countOf(selected.count)}</span>
          {/if}
        </h3>
        {#if selected.events.length === 0}
          <p class="text-sm text-muted-foreground">{s.panelNothing}</p>
        {:else}
          <ul class="flex flex-col gap-3">
            {#each selected.events as e (e.id)}
              <li class="flex gap-3">
                <span
                  class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-full border {eventTone(e.kind)}"
                  class:bg-current={e.observed}
                  style="border-color: currentColor"
                ></span>
                <div class="min-w-0 flex-1">
                  <p class="text-sm">
                    <span class="font-medium">{e.company_slug}</span>
                    {#if e.role_title}<span class="text-muted-foreground"> · {e.role_title}</span>{/if}
                  </p>
                  <p class="text-sm text-muted-foreground">{eventLabel(e)}</p>
                  {#if e.email_subject}
                    <p class="truncate text-sm italic text-muted-foreground">“{e.email_subject}”</p>
                  {/if}
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {#if e.observed}{timeOf(e.occurred_at)}{:else}{s.recordedByYou}{/if}
                    {#if boardRefFor(e)}
                      · <a class="underline hover:no-underline" href={resolve('/my/tracking/[id]', { id: boardRefFor(e) ?? '' })}
                        >{s.applicationLink}</a
                      >
                    {/if}
                    {#if e.email_id}
                      ·
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve()d base plus a query string; there is no dynamic route segment to resolve -->
                      <a class="underline hover:no-underline" href={`${resolve('/my/inbox')}?message=${e.email_id}`}
                        >{s.messageLink}</a
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

    {#if grid.total === 0}
      <div class="rounded-lg border bg-card p-4">
        <p class="text-sm text-muted-foreground">{s.empty}</p>
        <Button variant="outline" size="sm" class="mt-3" href={resolve('/my/tracking')}>{s.emptyCta}</Button>
      </div>
    {/if}
  {/if}
</div>
