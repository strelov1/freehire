<script lang="ts">
  import { resolve } from '$app/paths';
  import Seo from '$lib/components/Seo.svelte';
  import TalentCard from '$lib/components/TalentCard.svelte';
  import { CATEGORY_VALUES, SENIORITY_VALUES } from '$lib/generated/contracts';
  import { CATEGORY_LABELS, SENIORITY_LABELS, titleCase } from '$lib/labels';
  import {
    DEFAULT_LIMIT,
    TIMEZONE_REGIONS,
    YEAR_THRESHOLDS,
    talentFilterSearch,
    talentPageSearch,
    talentYearsSearch,
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
   *  reads: clicking a second value adds it, clicking a selected one removes it.
   *
   *  `labels` is an EXCEPTION table, not a full dictionary — SENIORITY_LABELS carries one
   *  entry — so an absent value falls through to titleCase, the same fallback facets.ts
   *  and insights.ts use. Reading it as complete renders chips as bare slugs. */
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
        label: labels[value] ?? titleCase(value),
        href: search ? `${resolve('/talent')}?${search}` : resolve('/talent'),
        selected: selected.includes(value),
      };
    });
  }

  const seniorityOptions = $derived(
    options(query, 'seniorities', SENIORITY_VALUES, SENIORITY_LABELS, query.seniorities ?? []),
  );

  // The discipline row filters on SPECIALIZATIONS — what the candidate ticked on their own
  // profile — not on the category derived from their job titles. Two rows over the same
  // 48-value vocabulary would be 96 chips on one screen, and of the two this is the one a
  // recruiter is actually asking about: where somebody wants to go, not where they have
  // been. Where they have been is on the card, and `categories` stays a URL/API filter.
  const specializationOptions = $derived(
    options(
      query,
      'specializations',
      CATEGORY_VALUES,
      CATEGORY_LABELS,
      query.specializations ?? [],
    ),
  );

  const timezoneOptions = $derived(
    options(query, 'tz', TIMEZONE_REGIONS, {}, query.tz ?? []),
  );

  // Years is a THRESHOLD, not a set: "at least this much". So the row is single-select —
  // clicking the active one clears it — rather than the toggle-into-a-set the others use.
  const yearOptions = $derived(
    YEAR_THRESHOLDS.map((years) => {
      const selected = query.minYears === years;
      const search = talentYearsSearch(query, selected ? undefined : years);
      return {
        value: String(years),
        label: `${years}+ years`,
        href: search ? `${resolve('/talent')}?${search}` : resolve('/talent'),
        selected,
      };
    }),
  );
</script>

<Seo
  title="Talent Network — freehire"
  description="Anonymous profiles of candidates open to being approached. Filter by discipline, seniority, timezone and experience."
/>
<svelte:head>
  <!-- noindex WHILE JOINING IS BETA-ONLY. The list is meant to be indexable — it is the
  front door of the feature and carries no personal data — but a catalogue whose entire
  membership is the beta group is not the catalogue we would want indexed, and a search
  result promising candidates that leads to four is worse than no result. Lift this in the
  same change that lifts the join gate. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 py-8">
  <header class="flex flex-col gap-2">
    <h1 class="text-2xl font-semibold tracking-tight">Talent Network</h1>
    <p class="max-w-2xl text-sm text-muted-foreground">
      Candidates who asked to be found, shown anonymously. No names, no employers, no
      contact details — what you see is what they chose to publish.
    </p>
  </header>

  <!-- The Chip primitive is display-only — no href, no selected state — so the anchor wraps
  it and the variant carries the selection. Wrapping rather than widening a shared component
  for one listing's filter rows; `primary` already reads as "on". -->
  <section class="flex flex-col gap-4" aria-label="Filters">
    <div class="flex flex-col gap-1.5">
      <h2 class="text-xs font-medium text-muted-foreground">Open to</h2>
      <div class="flex flex-wrap gap-1.5">
        {#each specializationOptions as option (option.value)}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- built by options() from resolve('/talent') plus a filter query; the rule cannot see through the appended search string -->
          <a href={option.href}>
            <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
          </a>
        {/each}
      </div>
    </div>

    <div class="flex flex-wrap gap-x-8 gap-y-4">
      <div class="flex flex-col gap-1.5">
        <h2 class="text-xs font-medium text-muted-foreground">Grade</h2>
        <div class="flex flex-wrap gap-1.5">
          {#each seniorityOptions as option (option.value)}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- same as the row above: resolve('/talent') plus a query string -->
            <a href={option.href}>
              <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
            </a>
          {/each}
        </div>
      </div>

      <div class="flex flex-col gap-1.5">
        <h2 class="text-xs font-medium text-muted-foreground">Experience</h2>
        <div class="flex flex-wrap gap-1.5">
          {#each yearOptions as option (option.value)}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve('/talent') plus a query string -->
            <a href={option.href}>
              <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
            </a>
          {/each}
        </div>
      </div>

      <div class="flex flex-col gap-1.5">
        <h2 class="text-xs font-medium text-muted-foreground">Timezone</h2>
        <div class="flex flex-wrap gap-1.5">
          {#each timezoneOptions as option (option.value)}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve('/talent') plus a query string -->
            <a href={option.href}>
              <Chip variant={option.selected ? 'primary' : 'default'}>{option.label}</Chip>
            </a>
          {/each}
        </div>
      </div>
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
