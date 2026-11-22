<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ArrowUpRight, CalendarDays, MessagesSquare, Mic } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { api } from '#lib/api.js';
  import { createInterviewSession, listSessions } from '#lib/assistant/api.js';
  import type { SessionSummary } from '#lib/assistant/wire.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import { boardRefFor } from '#lib/board.js';
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { ScheduledInterview } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { messages } from './messages';

  const dayMs = 24 * 60 * 60 * 1000;
  const windowMs = 90 * dayMs;
  const s = $derived(t(messages, locale()));
  const timeZone = $derived(currentUser()?.timezone || undefined);
  const dateTime = $derived(new Intl.DateTimeFormat(locale(), { dateStyle: 'medium', timeStyle: 'short', timeZone }));
  const dateOnly = $derived(new Intl.DateTimeFormat(locale(), { dateStyle: 'medium', timeZone: 'UTC' }));

  let now = $state(0);
  let meetings = $state.raw<ScheduledInterview[] | null>(null);
  let sessions = $state.raw<SessionSummary[] | null>(null);
  let meetingsFailed = $state(false);
  let sessionsFailed = $state(false);
  let starting = $state(false);
  let startErrorId = $state<number | null>(null);

  const confirmedOrSuggested = $derived(
    (meetings ?? []).filter((meeting) => meeting.status !== 'cancelled').sort(byStart),
  );
  const upcoming = $derived(confirmedOrSuggested.filter((meeting) => !hasEnded(meeting, now)));
  const past = $derived(confirmedOrSuggested.filter((meeting) => hasEnded(meeting, now)).reverse());

  function byStart(a: ScheduledInterview, b: ScheduledInterview) {
    return Date.parse(a.starts_at) - Date.parse(b.starts_at);
  }

  function hasEnded(meeting: ScheduledInterview, at: number) {
    const end = meeting.ends_at ? Date.parse(meeting.ends_at) : Date.parse(meeting.starts_at) + dayMs;
    return end <= at;
  }

  async function loadMeetings() {
    now = Date.now();
    meetingsFailed = false;
    try {
      meetings = await api.myInterviews(new Date(now - windowMs).toISOString(), new Date(now + windowMs).toISOString());
    } catch {
      meetingsFailed = true;
    }
  }

  async function loadSessions() {
    sessionsFailed = false;
    try {
      sessions = (await listSessions()).filter(
        (session) => session.preset === 'interview' || session.preset === 'debrief',
      );
    } catch {
      sessionsFailed = true;
    }
  }

  onMount(() => {
    void loadMeetings();
    void loadSessions();
    const timer = setInterval(() => (now = Date.now()), 60_000);
    return () => clearInterval(timer);
  });

  async function startSession(meeting: ScheduledInterview, preset: 'interview' | 'debrief') {
    if (!meeting.job_slug || starting) return;
    starting = true;
    startErrorId = null;
    try {
      const session = await createInterviewSession(preset, meeting.job_slug);
      await goto(resolve('/my/assistant/[[id]]', { id: session.id }));
    } catch {
      startErrorId = meeting.id;
    } finally {
      starting = false;
    }
  }
</script>

<svelte:head><title>{s.title} · freehire</title></svelte:head>

{#snippet meetingList(items: ScheduledInterview[], preset: 'interview' | 'debrief')}
  {#if items.length}
    <ul class="divide-y divide-border border-y border-border">
      {#each items as meeting (meeting.id)}
        {@const boardRef = boardRefFor(meeting)}
        <li class="grid gap-x-6 gap-y-2 py-4 sm:grid-cols-[10rem_minmax(0,1fr)_auto] sm:items-center">
          <time datetime={meeting.starts_at} class="text-sm tabular-nums text-muted-foreground">
            {meeting.ends_at
              ? dateTime.format(new Date(meeting.starts_at))
              : dateOnly.format(new Date(meeting.starts_at))}
          </time>
          <div class="min-w-0">
            <p class="break-words text-sm font-medium">{meeting.role_title || meeting.title || s.interview}</p>
            <p class="break-words text-sm text-muted-foreground">
              {meeting.company_slug}{#if meeting.role_title && meeting.title}
                · {meeting.title}{/if}{#if meeting.status === 'suggested'}
                · {s.unconfirmed}{/if}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            {#if boardRef}
              <a
                class="px-2 py-1 text-sm text-muted-foreground underline-offset-4 hover:underline"
                href={resolve('/my/tracking/[id]', { id: boardRef })}>{s.application}</a
              >
            {/if}
            {#if meeting.job_slug}
              <Button
                size="sm"
                variant={preset === 'interview' ? 'primary' : 'outline'}
                disabled={starting}
                onclick={() => startSession(meeting, preset)}
              >
                {#if preset === 'interview'}<Mic class="size-4" aria-hidden="true" />{/if}
                {preset === 'interview' ? s.prepare : s.debrief}
              </Button>
            {/if}
          </div>
          {#if startErrorId === meeting.id}
            <p role="alert" class="text-sm text-destructive sm:col-span-3">{s.startFailed}</p>
          {/if}
        </li>
      {/each}
    </ul>
  {:else}
    <p class="border-t border-border pt-4 text-sm text-muted-foreground">
      {preset === 'interview' ? s.emptyUpcoming : s.emptyPast}
    </p>
  {/if}
{/snippet}

<div class="flex flex-col gap-8">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <AccountPageHeading>{s.title}</AccountPageHeading>
    <Button size="sm" variant="outline" href={resolve('/my/tracking')}>{s.tracking}</Button>
  </div>

  <section aria-labelledby="upcoming-interviews" class="flex flex-col gap-3">
    <h2 id="upcoming-interviews" class="flex items-center gap-2 text-lg font-semibold">
      <CalendarDays class="size-5 text-brand" aria-hidden="true" />
      {s.upcoming}
    </h2>
    {#if meetingsFailed}
      <div class="flex flex-wrap items-center gap-3">
        <p role="alert" class="text-sm text-destructive">{s.loadMeetingsFailed}</p>
        <Button size="sm" variant="outline" onclick={loadMeetings}>{s.retry}</Button>
      </div>
    {:else if meetings === null}
      <p role="status" class="text-sm text-muted-foreground">{s.loading}</p>
    {:else}
      {@render meetingList(upcoming, 'interview')}
    {/if}
  </section>

  <section aria-labelledby="interview-history" class="flex flex-col gap-3">
    <h2 id="interview-history" class="flex items-center gap-2 text-lg font-semibold">
      <MessagesSquare class="size-5 text-brand" aria-hidden="true" />
      {s.history}
    </h2>
    {#if sessionsFailed}
      <div class="flex flex-wrap items-center gap-3">
        <p role="alert" class="text-sm text-destructive">{s.loadHistoryFailed}</p>
        <Button size="sm" variant="outline" onclick={loadSessions}>{s.retry}</Button>
      </div>
    {:else if sessions === null}
      <p role="status" class="text-sm text-muted-foreground">{s.loading}</p>
    {:else if sessions.length}
      <ul class="divide-y divide-border border-y border-border">
        {#each sessions as session (session.id)}
          <li>
            <a
              href={resolve('/my/assistant/[[id]]', { id: session.id })}
              class="flex items-center justify-between gap-3 py-3 pl-1 pr-2 text-sm hover:bg-accent/50"
            >
              <span class="min-w-0 break-words font-medium">{session.label || s.sessionFallback}</span>
              <span class="flex shrink-0 items-center gap-2 text-muted-foreground">
                {session.preset === 'interview' ? s.practiceType : s.debriefType}
                <ArrowUpRight class="size-4" aria-hidden="true" />
              </span>
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="border-t border-border pt-4 text-sm text-muted-foreground">{s.emptyHistory}</p>
    {/if}
  </section>

  {#if meetings !== null && !meetingsFailed}
    <section aria-labelledby="past-interviews" class="flex flex-col gap-3">
      <h2 id="past-interviews" class="text-lg font-semibold">{s.past}</h2>
      {@render meetingList(past, 'debrief')}
    </section>
  {/if}
</div>
