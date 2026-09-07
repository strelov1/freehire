<script lang="ts">
  import { flip } from 'svelte/animate';
  import { fly } from 'svelte/transition';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { companyLogoUrl } from '$lib/logo';
  import {
    aggregateLabel,
    companyAggregateLabel,
    pushFeedEntry,
    type RecentFeedEntry,
    type RecentFeedEvent,
  } from '$lib/recentFeed';
  import { EntityLogo, SectionLabel } from '$lib/ui';
  import { timeAgo } from '$lib/utils';

  // The homepage's live "recently added jobs" feed (see
  // openspec/changes/add-homepage-recent-jobs-feed). Renders nothing until the first
  // event arrives — a visibly empty section would read as broken, not as "nothing
  // yet". The stream itself is public and needs no credentials.
  const MAX_ENTRIES = 8;

  let entries = $state<RecentFeedEntry[]>([]);
  let nextId = 0;

  // A millisecond clock, ticked once a second so the "N ago" label on each
  // card advances live instead of freezing at whatever it read on arrival.
  // Reading `now` from ago() below — called from the template — is what
  // makes that call re-run on every tick; no separate signal needed.
  let now = $state(Date.now());
  $effect(() => {
    const interval = setInterval(() => (now = Date.now()), 1000);
    return () => clearInterval(interval);
  });

  function ago(entry: RecentFeedEntry): string {
    const producedMs = Date.parse(entry.produced_at);
    if (Number.isNaN(producedMs)) return '';
    // Clamp a produced_at that reads as being in the client's future (clock
    // skew between this browser and the server) to `now`, so the newest card
    // never renders as "in 2 seconds" instead of "just now".
    return timeAgo(new Date(Math.min(producedMs, now)).toISOString());
  }

  function headline(entry: RecentFeedEntry): string {
    // A company_aggregate has no single featured role — the company itself
    // is the headline, mirroring how a role-aggregate headlines the role.
    return entry.kind === 'company_aggregate' ? entry.company_name : entry.title;
  }

  function subtitle(entry: RecentFeedEntry): string {
    switch (entry.kind) {
      case 'single':
        return entry.company_name;
      case 'aggregate':
        return aggregateLabel(entry);
      case 'company_aggregate':
        return companyAggregateLabel(entry);
    }
  }

  $effect(() => {
    const source = new EventSource(api.recentJobsFeedUrl());
    source.addEventListener('job', (e) => {
      let data: RecentFeedEvent;
      try {
        data = JSON.parse((e as MessageEvent).data);
      } catch {
        return; // a malformed frame is dropped rather than breaking the feed
      }
      nextId += 1;
      entries = pushFeedEntry(entries, data, nextId, MAX_ENTRIES);
    });
    // EventSource retries on its own; a dropped connection needs no handling here.
    return () => source.close();
  });

  function entryHref(entry: RecentFeedEntry): string {
    if (entry.kind === 'single' && entry.job_slug) {
      return resolve('/jobs/[slug]', { slug: entry.job_slug });
    }
    // Either aggregate kind represents several postings — there is no single
    // job to link to, so it points at the catalogue instead.
    return resolve('/jobs');
  }
</script>

{#if entries.length > 0}
  <section class="border-t border-border py-10">
    <SectionLabel text="just added" />
    <ul class="mt-6 flex flex-col gap-2">
      {#each entries as entry (entry.id)}
        <li animate:flip={{ duration: 200 }}>
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- entryHref always returns a resolve() result; the rule cannot see through the function call -->
          <a href={entryHref(entry)}
            in:fly={{ y: -8, duration: 200 }}
            class="flex items-center gap-3 rounded-lg border border-border p-3 transition-colors hover:border-brand-strong hover:bg-accent"
          >
            <EntityLogo
              name={entry.company_name}
              src={companyLogoUrl(entry.company_name) ?? undefined}
              shape="square"
              size="sm"
            />
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-medium">{headline(entry)}</p>
              <p class="truncate text-xs text-muted-foreground">{subtitle(entry)}</p>
            </div>
            <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{ago(entry)}</span>
          </a>
        </li>
      {/each}
    </ul>
  </section>
{/if}
