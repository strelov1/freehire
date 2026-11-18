<script lang="ts">
  import { eventLabel, eventTone } from '#lib/events.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { TimelineEvent } from '#lib/types.js';
  import { timeAgo } from '#lib/utils.js';
  import { messages } from './JobDrawer.messages';

  // The drawer's History tab once its events are loaded: the application's timeline.
  let { events }: { events: TimelineEvent[] } = $props();

  const s = $derived(t(messages, locale()));
</script>

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
