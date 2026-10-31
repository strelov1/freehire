<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { buddySharedCompanies } from '#lib/buddyPresent.js';
  import type { BuddySimilar } from '#lib/types.js';
  import BuddyIdentity from './BuddyIdentity.svelte';

  // A short "people like you" list for a page's right rail, linking to the full network.
  // A suggestion, not the page's content: a failed read leaves it out entirely.
  let { exclude = '', limit = 4 }: { exclude?: string; limit?: number } = $props();

  let people = $state<BuddySimilar[]>([]);

  $effect(() => {
    const skip = exclude;
    let cancelled = false;
    people = [];
    api.listBuddySimilar(limit + 1).then(
      (s) => { if (!cancelled) people = s.filter((m) => m.handle !== skip).slice(0, limit); },
      () => {},
    );
    return () => { cancelled = true; };
  });
</script>

{#if people.length}
  <section class="rounded-xl border border-border bg-card p-4">
    <div class="mb-1 flex items-baseline justify-between">
      <h2 class="text-sm font-semibold">People like you</h2>
      <a href={resolve('/buddies/network')} class="text-xs font-medium text-brand-strong hover:underline">See all</a>
    </div>
    <ul class="flex flex-col divide-y divide-border">
      {#each people as person (person.handle)}
        <li class="flex flex-col gap-0.5 py-2.5">
          <BuddyIdentity handle={person.handle} summary={person.card_summary} size="sm" />
          {#if person.shared_companies.length}
            <span class="pl-11 text-xs text-muted-foreground">{buddySharedCompanies(person.shared_companies.length)}</span>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
