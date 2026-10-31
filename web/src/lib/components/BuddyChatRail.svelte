<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { buddyHeading } from '#lib/buddyPresent.js';
  import { countryLabel } from '#lib/facets.js';
  import type { BuddyCompany, BuddyFeedItem, BuddySummary } from '#lib/types.js';
  import { Button, CountryFlag } from '#lib/ui/index.js';
  import BuddyAvatar from './BuddyAvatar.svelte';
  import BuddyCompanyLogo from './BuddyCompanyLogo.svelte';
  import BuddyFeedEntry from './BuddyFeedEntry.svelte';

  // Who the open chat is with, beside it: their card, the companies both of you applied
  // to, and their latest moves — what to talk about. Only where the chat keeps room to read
  // (2xl); below that the chat's own "you both applied to" carries the context.
  let { handle, summary, companies }: { handle: string; summary: BuddySummary | null; companies: BuddyCompany[] } = $props();

  const LATEST = 3;
  let latest = $state<BuddyFeedItem[]>([]);

  $effect(() => {
    const current = handle;
    let cancelled = false;
    latest = [];
    // Their own feed, under every rule the feed keeps. A failure leaves the section out.
    api.listBuddyMemberFeed(current, undefined, LATEST).then(
      (page) => { if (!cancelled) latest = page.data; },
      () => {},
    );
    return () => { cancelled = true; };
  });
</script>

<aside class="hidden shrink-0 flex-col gap-3 overflow-y-auto 2xl:flex 2xl:w-68" aria-label="About {handle}">
  <section class="flex flex-col items-center gap-1 rounded-xl border border-border bg-card p-5 text-center">
    <BuddyAvatar {handle} size="lg" class="mb-1 size-16 text-xl" />
    {#if summary}
      <p class="flex items-center gap-1.5 font-semibold">
        {buddyHeading(summary)}
        {#if summary.country}<CountryFlag code={summary.country} label={countryLabel(summary.country)} class="text-xs" />{/if}
      </p>
    {/if}
    <p class="text-xs text-muted-foreground">{handle}</p>
    <Button href={resolve('/buddies/[handle]', { handle })} variant="outline" size="sm" class="mt-2">View profile</Button>
  </section>

  {#if companies.length}
    <section class="rounded-xl border border-border bg-card p-4">
      <h2 class="mb-2 text-sm font-semibold">You both applied to</h2>
      <ul class="flex flex-col gap-1">
        {#each companies as c (c.slug)}
          <li>
            <a href={resolve('/companies/[slug]', { slug: c.slug })} class="flex items-center gap-2.5 rounded-md p-1 text-sm font-medium hover:bg-muted">
              <BuddyCompanyLogo name={c.name} size="sm" />{c.name}
            </a>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  {#if latest.length}
    <section class="rounded-xl border border-border bg-card px-4 pt-3 pb-1">
      <h2 class="text-sm font-semibold">Their latest</h2>
      <ul>{#each latest as item (item.id)}<BuddyFeedEntry {item} />{/each}</ul>
    </section>
  {/if}
</aside>
