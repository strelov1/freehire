<script lang="ts">
  import { flip } from 'svelte/animate';
  import { fly } from 'svelte/transition';
  import { prefersReducedMotion } from 'svelte/motion';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import {
    aggregateLabel,
    companyAggregateLabel,
    pushFeedEntry,
    type RecentFeedEntry,
    type RecentFeedEvent,
  } from '#lib/recentFeed.js';
  import { tick, untrack } from 'svelte';
  import { Pause, Play } from '@lucide/svelte';
  import { EntityLogo, SectionLabel } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { timeAgo } from '#lib/utils.js';

  // The homepage's live "recently added jobs" feed (see
  // openspec/changes/add-homepage-recent-jobs-feed). Renders nothing until the first
  // event arrives — a visibly empty section would read as broken, not as "nothing
  // yet". The stream itself is public and needs no credentials.
  const MAX_ENTRIES = 8;

  let entries = $state<RecentFeedEntry[]>([]);
  let nextId = 0;

  // A list that rewrites itself every few seconds is moving content, and WCAG 2.2.2 asks
  // for a way to stop it. Three things hold it still: the visitor's own Pause, a pointer
  // over the list, and keyboard focus inside it — the last two because a card is hard to
  // read, and impossible to click, while it slides away. Arrivals during a hold queue up
  // and land together when it ends, so a pause loses nothing.
  let paused = $state(false);
  let hovering = $state(false);
  let focusWithin = $state(false);
  const held = $derived(paused || hovering || focusWithin);
  let pending = $state<RecentFeedEvent[]>([]);
  // How many arrived during the hold. Counted apart from `pending`, which keeps only the
  // newest MAX_ENTRIES: the Resume label promises the true number, not the queue's cap.
  let arrivedWhileHeld = $state(0);
  let listEl = $state<HTMLUListElement>();

  async function land(events: RecentFeedEvent[]) {
    // Focus inside the list holds it, so a focused card should never be the one pushed
    // out — but if it is, focus goes to the list rather than falling back to the page.
    const hadFocus = !!listEl && listEl.contains(document.activeElement);
    for (const data of events) {
      nextId += 1;
      entries = pushFeedEntry(entries, data, nextId, MAX_ENTRIES);
    }
    if (!hadFocus) return;
    await tick();
    if (listEl && !listEl.contains(document.activeElement)) listEl.focus();
  }

  $effect(() => {
    if (held || pending.length === 0) return;
    const queued = pending;
    pending = [];
    arrivedWhileHeld = 0;
    // Untracked: `land` reads `entries` to extend it, which must not make this effect
    // depend on the list it writes.
    untrack(() => land(queued));
  });

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
    return timeAgo(new Date(Math.min(producedMs, now)).toISOString(), locale());
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
      // Only the newest MAX_ENTRIES can ever be shown, so the queue keeps no more.
      if (held) {
        pending = [...pending, data].slice(-MAX_ENTRIES);
        arrivedWhileHeld += 1;
      } else land([data]);
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

  // Reduced motion: entries still arrive and reorder, just without the slide.
  const motionMs = $derived(prefersReducedMotion.current ? 0 : 200);
</script>

{#if entries.length > 0}
  <section class="border-t border-border py-10">
    <div class="flex items-center justify-between gap-3">
      <SectionLabel as="h2" text="just added" />
      <button
        type="button"
        onclick={() => (paused = !paused)}
        class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 font-mono text-xs text-muted-foreground transition-colors hover:text-foreground"
      >
        {#if paused}
          <Play class="size-3.5" aria-hidden="true" />
          Resume{arrivedWhileHeld > 0 ? ` (${arrivedWhileHeld} new)` : ''}
        {:else}
          <Pause class="size-3.5" aria-hidden="true" />
          Pause
        {/if}
      </button>
    </div>
    <!-- Every slot is laid out from the first arrival: the cards still to come stand as
         invisible placeholders of the same height, so a new arrival replaces one instead
         of pushing the rest of the page down by a card each time. `tabindex="-1"` makes
         the list itself somewhere focus can go (see `land`). -->
    <ul
      bind:this={listEl}
      tabindex="-1"
      aria-label="Recently added jobs"
      class="mt-6 flex flex-col gap-2 outline-none"
      onpointerenter={() => (hovering = true)}
      onpointerleave={() => (hovering = false)}
      onfocusin={() => (focusWithin = true)}
      onfocusout={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) focusWithin = false;
      }}
    >
      {#each entries as entry (entry.id)}
        <li animate:flip={{ duration: motionMs }}>
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- entryHref always returns a resolve() result; the rule cannot see through the function call -->
          <a href={entryHref(entry)}
            in:fly={{ y: -8, duration: motionMs }}
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
      {#each { length: MAX_ENTRIES - entries.length }, i (i)}
        <!-- The card's box with nothing in it: same padding, border, logo square and two
             text lines, so the height matches to the pixel. -->
        <li aria-hidden="true" class="invisible">
          <div class="flex items-center gap-3 rounded-lg border p-3">
            <div class="size-8 shrink-0"></div>
            <div class="min-w-0 flex-1">
              <p class="text-sm">&nbsp;</p>
              <p class="text-xs">&nbsp;</p>
            </div>
          </div>
        </li>
      {/each}
    </ul>
  </section>
{/if}
