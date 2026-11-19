<script lang="ts">
  import { eventLabel, eventTone } from '#lib/events.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { ScheduledInterview, TimelineEvent } from '#lib/types.js';
  import { timeAgo } from '#lib/utils.js';
  import JobDrawerBookingLink from './JobDrawerBookingLink.svelte';
  import { messages } from './JobDrawer.messages';

  // The drawer's History tab once its events are loaded: the booking link, the meetings
  // arranged for the application, and its timeline.
  let {
    events,
    interviews = [],
    applicationId = null,
    bookingUrl = $bindable(null),
  }: {
    events: TimelineEvent[];
    interviews?: ScheduledInterview[];
    applicationId?: number | null;
    bookingUrl?: string | null;
  } = $props();

  const s = $derived(t(messages, locale()));

  const when = (iso: string) =>
    new Date(iso).toLocaleString(locale(), {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    });
</script>

{#if applicationId}
  <div class="mb-4">
    <JobDrawerBookingLink {applicationId} bind:url={bookingUrl} />
  </div>
{/if}

{#if interviews.length}
  <section class="mb-4 flex flex-col gap-1.5 text-sm">
    <h3 class="font-medium">{s.bookingLink.meetings}</h3>
    <ul class="flex flex-col gap-1">
      {#each interviews as iv (iv.id)}
        <li
          class="flex flex-col sm:flex-row sm:justify-between sm:gap-3"
          class:line-through={iv.status === 'cancelled'}
        >
          <span class="min-w-0">{iv.title || iv.company_slug}</span>
          <time datetime={iv.starts_at} class="text-xs text-muted-foreground sm:whitespace-nowrap"
            >{when(iv.starts_at)}</time
          >
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if events.length}
  <div class="flex flex-col gap-1 text-sm">
    <ol class="flex flex-col gap-1.5">
      {#each events as e (e.id)}
        <!-- The event first and its age beneath it on a phone, where a fixed time
             column wrapped "11 секунд назад" onto two lines beside a one-line
             event; side by side from sm up, the age kept whole on the right. -->
        <li class="flex items-baseline gap-2">
          <span class="shrink-0 text-xs {eventTone(e.kind)}" aria-hidden="true">●</span>
          <div class="flex min-w-0 flex-1 flex-col sm:flex-row sm:items-baseline sm:justify-between sm:gap-3">
            <span class="min-w-0 text-sm">{eventLabel(e, locale())}</span>
            <time datetime={e.occurred_at} class="text-xs text-muted-foreground sm:shrink-0 sm:whitespace-nowrap">
              {timeAgo(e.occurred_at, locale())}
            </time>
          </div>
        </li>
      {/each}
    </ol>
  </div>
{:else}
  <p class="text-sm text-muted-foreground">{s.noHistory}</p>
{/if}
