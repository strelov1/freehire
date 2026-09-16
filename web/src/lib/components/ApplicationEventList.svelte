<script lang="ts">
  import { resolve } from '$app/paths';
  import { boardRefFor } from '$lib/board';
  import { eventLabel, eventTone } from '$lib/events';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import type { TimelineEvent } from '$lib/types';
  import { messages } from './ApplicationEventList.messages';

  // One day's ledger events, listed. Shared by the two day panels that show them — the
  // calendar's and the activity grid's.
  //
  // It exists for the reason `$lib/events` gives one layer down: that module holds the labels
  // and tones because "copying them would have meant the same event captioned two ways on two
  // screens". The MARKUP around those labels had the same problem and was duplicated anyway —
  // the dot whose fill carries `observed`, the company/role line, the quoted subject, the
  // clock and the two links — so it lives here now and neither panel owns a copy.
  //
  // Presentational only: it fetches nothing, and in particular never touches the message
  // endpoint, which marks mail read.
  let { events }: { events: TimelineEvent[] } = $props();

  const s = $derived(t(messages, locale()));

  const timeOf = (instant: string) =>
    new Date(instant).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' });
</script>

<ul class="flex flex-col gap-3">
  {#each events as e (e.id)}
    <li class="flex gap-3">
      <!-- Filled = a date somebody other than the candidate set. Hollow = one they recorded
           themselves. Shape and fill rather than hue alone, so the difference survives being
           seen without colour. -->
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
