<script lang="ts">
  import { onMount } from 'svelte';
  import { goto, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/utils';
  import { signinUrl } from '$lib/signin';
  import { Badge, Button, Card, Skeleton } from '$lib/ui';
  import {
    addMonths,
    browserTimezone,
    daysWithSlots,
    groupSlotsByLocalDay,
    monthGrid,
    monthOf,
    slotLocalTime,
    slotWindowForMonth,
    todayIn,
    weekdayOrder,
    weekdayShortLabel,
    withSearchParams,
  } from '$lib/mentorship';
  import type { Mentor, MentorSlot } from '$lib/types';

  let { mentor }: { mentor: Mentor } = $props();

  // How often an open page re-asks for the slots. Derived, not chosen: the endpoint caches
  // a computed window for one minute, so a faster poll is answered from the same entry and
  // spends the visitor's share of a rate limit for an identical answer. cal.com polls every
  // five minutes and holds a reservation system besides; we need neither, because the
  // database refuses the second booking outright.
  const REFRESH_MS = 60_000;

  // The zone the browser believes it is in. What the times are ACTUALLY in is whatever the
  // response reports — an unrecognised name is answered in UTC — so this is only ever the
  // request, never the label.
  const browserZone = browserTimezone();

  let slots = $state<MentorSlot[]>([]);
  let zone = $state(browserZone);
  let loading = $state(true);
  let failed = $state(false);

  // ?month, ?date and ?slot, exactly as cal.com does it. That is what survives the sign-in
  // redirect a signed-out seeker is about to take — a slot held only in this tab's memory
  // does not, and recovering it from sessionStorage is a second mechanism to debug on
  // somebody else's browser.
  //
  // But the selection is held HERE and only mirrored to the address bar, rather than being
  // read back out of it. `page.url` lags a shallow `replaceState` (urlSynced.svelte.ts
  // records the same thing), so deriving from it means the address updates and the screen
  // does not — the control ends up describing a day the page is not showing.
  let selection = $state({
    month: page.url.searchParams.get('month') ?? '',
    date: page.url.searchParams.get('date') ?? '',
    slot: page.url.searchParams.get('slot') ?? '',
  });

  const month = $derived(selection.month || monthOf(todayIn(zone)));
  const selectedDay = $derived(selection.date);
  const selectedSlot = $derived(selection.slot);

  const byDay = $derived(groupSlotsByLocalDay(slots));
  const open = $derived(daysWithSlots(slots));
  const weeks = $derived(monthGrid(month));
  const daySlots = $derived(byDay.get(selectedDay) ?? []);

  // Shallow routing, not `goto`: the slots are fetched in the browser, so re-running the
  // route's `load` on every click of a day would be a server round trip that changes
  // nothing on the page. `replaceState` also keeps the back button meaning "the page
  // before this one" rather than "the previous day I looked at".
  //
  // Only ever called from an event handler. In `onMount` this throws — and only in a
  // production build, where the explanatory message is compiled out.
  function setParams(next: { month?: string; date?: string | null; slot?: string | null }) {
    selection = {
      month: next.month ?? selection.month,
      date: next.date === undefined ? selection.date : (next.date ?? ''),
      slot: next.slot === undefined ? selection.slot : (next.slot ?? ''),
    };
    const query = withSearchParams(page.url.searchParams, selection);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(page.url.pathname + (query ? `?${query}` : ''), {});
  }

  // Picking a day drops the slot chosen under the previous one: keeping it would leave the
  // page showing one date and about to book another.
  const pickDay = (day: string) => setParams({ date: day, slot: null });
  const pickSlot = (slot: MentorSlot) => setParams({ slot: slot.starts_at });
  const stepMonth = (delta: number) =>
    setParams({ month: addMonths(month, delta), date: null, slot: null });

  // Browser back/forward over our own shallow entries. Read the address bar rather than
  // `page.url`, which lags exactly here — the same reason urlSynced.svelte.ts does.
  function reseedFromAddressBar() {
    const params = new URLSearchParams(location.search);
    selection = {
      month: params.get('month') ?? '',
      date: params.get('date') ?? '',
      slot: params.get('slot') ?? '',
    };
  }

  let inflight: AbortController | null = null;

  async function load(window: { from: string; to: string }) {
    inflight?.abort();
    const controller = new AbortController();
    inflight = controller;
    loading = true;
    try {
      const res = await api.getMentorSlots(
        mentor.slug,
        window.from,
        window.to,
        browserZone,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      slots = res.slots;
      // The zone the server used, which may not be the one asked for.
      zone = res.timezone;
      failed = false;
    } catch {
      if (!controller.signal.aborted) failed = true;
    } finally {
      if (!controller.signal.aborted) loading = false;
    }
  }

  // One name for "ask again for the month on screen", said once rather than at each of
  // the four places that need it: first render, the timer, the tab regaining focus, and a
  // booking that was refused because the hour had gone.
  const reload = () => void load(slotWindowForMonth(month));

  $effect(reload);

  // Re-ask on the two occasions the shown hours can have gone stale without this tab
  // noticing: time simply passing, and the visitor coming back to a page they left open.
  // A taken hour may be offered for up to the cache's minute, and booking it is refused
  // with a reason — but the refusal is a worse way to learn it than the slot quietly
  // leaving the list.
  onMount(() => {
    const timer = setInterval(reload, REFRESH_MS);
    const onVisible = () => {
      if (document.visibilityState === 'visible') reload();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('popstate', reseedFromAddressBar);
    return () => {
      clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('popstate', reseedFromAddressBar);
      inflight?.abort();
    };
  });

  // ---- taking the hour ----------------------------------------------------------

  const signedIn = $derived(Boolean(page.data.user));
  const chosen = $derived(daySlots.find((s) => s.starts_at === selectedSlot));

  let note = $state('');
  let booking = $state(false);
  let bookingError = $state('');

  // Not promptSignIn(): that reads `page.url.search`, which lags our shallow writes, so a
  // visitor would return from signing in with the slot no longer chosen — the one thing
  // putting it in the URL was for. Read the address bar, which is current.
  function signInAndComeBack() {
    const returnTo = location.pathname + location.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo, mode: 'login' }));
  }

  async function book() {
    if (!chosen) return;
    booking = true;
    bookingError = '';
    try {
      const session = await api.bookMentorSession(mentor.slug, {
        // The absolute instant, never the wall clock: this is what the server re-derives
        // the slot from.
        starts_at: chosen.starts_at,
        // Recorded on the booking so the confirmation is written in the zone the seeker
        // actually booked in — `zone`, which is what the server used, not what we asked.
        timezone: zone,
        note,
      });
      void goto(resolve('/my/mentorship/sessions/[id]', { id: session.id }));
    } catch (e) {
      // The hour may simply have gone: somebody else took it, or the mentor moved their
      // availability inside the minute the slot cache holds. Re-ask rather than leaving a
      // list that still offers it.
      bookingError = errorMessage(e, 'That hour could not be booked.');
      reload();
    } finally {
      booking = false;
    }
  }

  // From the module that owns the Monday-first order, not a second list beside it.
  const WEEKDAYS = weekdayOrder().map(weekdayShortLabel);
  const monthLabel = $derived(
    new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(
      new Date(`${month}-01T00:00:00Z`),
    ),
  );
</script>

<Card class="p-4">
  <div class="flex flex-col gap-4 md:flex-row md:gap-8">
    <!-- The month -->
    <div class="md:w-72">
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

      <div class="mt-1 grid grid-cols-7 gap-1">
        {#each weeks as week, w (w)}
          {#each week as day, d (day || `blank-${w}-${d}`)}
            {#if day === ''}
              <span></span>
            {:else}
              <button
                type="button"
                disabled={!open.has(day)}
                aria-pressed={selectedDay === day}
                class="aspect-square rounded text-sm disabled:opacity-30
                       {selectedDay === day
                  ? 'bg-primary text-primary-foreground'
                  : open.has(day)
                    ? 'hover:bg-muted font-medium'
                    : ''}"
                onclick={() => pickDay(day)}
              >
                {Number(day.slice(8, 10))}
              </button>
            {/if}
          {/each}
        {/each}
      </div>

      {#if loading && slots.length === 0}
        <Skeleton class="mt-3 h-4 w-32" />
      {/if}
    </div>

    <!-- The day -->
    <div class="flex-1">
      {#if failed}
        <p class="text-muted-foreground text-sm">
          We couldn't load the available hours just now. They'll reappear on the next refresh.
        </p>
      {:else if !selectedDay}
        <p class="text-muted-foreground text-sm">Pick a day to see the hours it offers.</p>
      {:else if daySlots.length === 0}
        <p class="text-muted-foreground text-sm">No hours left on this day.</p>
      {:else}
        <p class="mb-2 text-sm font-medium">{selectedDay}</p>
        <ul class="flex flex-col gap-2">
          {#each daySlots as slot (slot.starts_at)}
            <li>
              <button
                type="button"
                aria-pressed={selectedSlot === slot.starts_at}
                class="flex w-full items-center justify-between rounded-md border px-3 py-2 text-sm
                       {selectedSlot === slot.starts_at
                  ? 'border-primary bg-primary/5'
                  : 'border-input hover:bg-muted'}"
                onclick={() => pickSlot(slot)}
              >
                <span class="font-medium">{slotLocalTime(slot)}</span>
                <!-- The offset is not decoration. On the autumn transition two slots carry
                     the same wall clock and differ only here; without it one October evening
                     shows the same hour twice with no way to choose. -->
                <Badge variant="secondary">UTC{slot.utc_offset}</Badge>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  {#if chosen}
    <div class="mt-4 border-t pt-4">
      <p class="text-sm">
        <span class="font-medium">{selectedDay} at {slotLocalTime(chosen)}</span>
        <span class="text-muted-foreground">
          (UTC{chosen.utc_offset}) · {mentor.session_minutes} minutes with {mentor.name}
        </span>
      </p>

      {#if signedIn}
        <label class="mt-3 block text-sm">
          <span class="text-muted-foreground">What would you like to talk about? (optional)</span>
          <textarea
            bind:value={note}
            rows="3"
            maxlength="1000"
            class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
            placeholder="A sentence is plenty — it helps them prepare."
          ></textarea>
        </label>

        <Button class="mt-3" disabled={booking} onclick={book}>
          {booking ? 'Booking…' : 'Book this session'}
        </Button>
      {:else}
        <p class="text-muted-foreground mt-3 text-sm">
          Sign in to book. Your chosen hour is in the address, so it will still be here.
        </p>
        <Button class="mt-3" onclick={signInAndComeBack}>Sign in to book</Button>
      {/if}

      {#if bookingError}
        <p class="text-destructive mt-2 text-sm">{bookingError}</p>
      {/if}
    </div>
  {/if}

  <p class="text-muted-foreground mt-4 text-xs">
    Times shown in {zone}. Sessions run {mentor.session_minutes} minutes.
  </p>
</Card>
