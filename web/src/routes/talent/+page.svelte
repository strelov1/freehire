<script lang="ts">
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import TalentCard from '$lib/components/TalentCard.svelte';
  import { CATEGORY_VALUES, SENIORITY_VALUES } from '$lib/generated/contracts';
  import { CATEGORY_LABELS, SENIORITY_LABELS } from '$lib/labels';
  import {
    DEFAULT_LIMIT,
    talentFilterSearch,
    talentPageSearch,
    type TalentFilterKey,
    type TalentQuery,
  } from '$lib/talentQuery';
  import { Chip, EmptyState } from '$lib/ui';
  import type { PageData } from './$types';

  // The public Talent Network catalogue: candidates who asked to be found, shown
  // anonymously. Every control here is an `<a href>` that navigates, never a local-state
  // toggle — the URL is what the loader reads, so a filtered catalogue is a shareable
  // link and a crawler sees what a visitor sees.

  let { data }: { data: PageData } = $props();

  const query = $derived(data.query);
  const members = $derived(data.page.items);
  const total = $derived(data.page.total ?? members.length);

  const limit = $derived(query.limit ?? DEFAULT_LIMIT);
  const offset = $derived(query.offset ?? 0);
  const hasPrev = $derived(offset > 0);
  const hasNext = $derived(data.page.hasMore);

  // Every href is built here, with resolve() spelled out in the expression. Hiding the
  // join in a helper would work at runtime and defeat both the router's type checking and
  // the lint rule that enforces it — the only thing standing between a renamed route and
  // a page full of dead links.
  const prevHref = $derived(
    `${resolve('/talent')}?${talentPageSearch(query, Math.max(0, offset - limit))}`,
  );
  const nextHref = $derived(`${resolve('/talent')}?${talentPageSearch(query, offset + limit)}`);

  interface FilterOption {
    value: string;
    label: string;
    href: string;
    selected: boolean;
  }

  /** One filter row: every value in the vocabulary, each with the href that toggles it.
   *
   *  Toggling rather than replacing is what makes a row behave like a set, which is how it
   *  reads: clicking a second discipline adds it, clicking a selected one removes it. */
  function options(
    q: TalentQuery,
    key: TalentFilterKey,
    values: readonly string[],
    labels: Record<string, string>,
    selected: readonly string[],
  ): FilterOption[] {
    return values.map((value) => {
      const next = selected.includes(value)
        ? selected.filter((v) => v !== value)
        : [...selected, value];
      const search = talentFilterSearch(q, key, next);
      return {
        value,
        label: labels[value] ?? value,
        href: search ? `${resolve('/talent')}?${search}` : resolve('/talent'),
        selected: selected.includes(value),
      };
    });
  }

  const seniorityOptions = $derived(
    options(query, 'seniorities', SENIORITY_VALUES, SENIORITY_LABELS, query.seniorities ?? []),
  );
  // The whole vocabulary is offered rather than the categories present today: a filter row
  // that shrank as members left would make the catalogue look like it had never covered
  // those disciplines.
  const categoryOptions = $derived(
    options(query, 'categories', CATEGORY_VALUES, CATEGORY_LABELS, query.categories ?? []),
  );
</script>

<Seo
  title="Talent Network — freehire"
  description="Anonymous profiles of candidates open to being approached. Filter by discipline, seniority, skills and timezone."
/>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-8">
  <header class="flex flex-col gap-2">
    <h1 class="text-2xl font-semibold tracking-tight">Talent Network</h1>
    <p class="max-w-2xl text-sm text-muted-foreground">
      Candidates who asked to be found, shown anonymously. No names, no employers, no
      contact details — what you see is what they chose to publish.
    </p>
  </header>

  <section class="flex flex-col gap-3" aria-label="Filters">
    <!-- The Chip primitive is display-only — no href, no selected state — so the anchor
    wraps it and the variant carries the selection. Wrapping rather than widening a shared
    component for one listing's filter row; `primary` already reads as "on". -->
    <div class="flex flex-wrap gap-1.5">
      {#each seniorityOptions as option (option.value)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- built by options() from resolve('/talent') plus a filter query; the rule cannot see through the appended search string -->
        <a href={option.href}>
          <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
        </a>
      {/each}
    </div>
    <div class="flex flex-wrap gap-1.5">
      {#each categoryOptions as option (option.value)}
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- same as the seniority row above: resolve('/talent') plus a query string -->
        <a href={option.href}>
          <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
        </a>
      {/each}
    </div>
    {#if query.tz?.length}
      <!-- Stated rather than left to be discovered: a timezone filter drops everyone whose
      zone is unknown, which otherwise reads as a small talent pool rather than a missing
      field. -->
      <p class="text-xs text-muted-foreground">
        Filtering by timezone hides candidates who have not set one.
      </p>
    {/if}
  </section>

  {#if members.length === 0}
    <EmptyState
      title="Nobody matches yet"
      description="Try widening the filters — the catalogue grows as candidates join."
    />
  {:else}
    <p class="text-sm text-muted-foreground">
      {total}
      {total === 1 ? 'candidate' : 'candidates'}
    </p>

    <div class="flex flex-col gap-3">
      <!-- Keyed by handle: it is unique per member and stable across pages, unlike an
      index, which would make Svelte reuse a card for a different person on navigation. -->
      {#each members as member (member.handle)}
        <TalentCard {member} />
      {/each}
    </div>

    <!-- Paging as real links, not the design system's Pager: its own doc comment says it
    is a local-state stepper that does not touch the URL, which is the opposite of what a
    server-rendered, shareable listing needs. -->
    {#if hasPrev || hasNext}
      <nav class="flex items-center justify-between gap-4" aria-label="Pagination">
        {#if hasPrev}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- prevHref/nextHref are resolve('/talent') plus the paging query -->
          <a class="text-sm hover:underline" href={prevHref}>← Previous</a>
        {:else}
          <span></span>
        {/if}
        {#if hasNext}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- see the Previous link above -->
          <a class="text-sm hover:underline" href={nextHref}>Next →</a>
        {/if}
      </nav>
    {/if}
  {/if}
</div>
