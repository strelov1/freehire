<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ArrowRight, Search } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import CollectionRowMark from '#lib/components/CollectionRowMark.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const canonical = $derived(`${page.url.origin}/collections`);

  // How many rows a section shows before "View all".
  const PREVIEW = 5;

  // Filters rows in place by title or slug. Every row is rendered and the ones past
  // the preview only hidden, so a search reaches the whole registry; a match is shown
  // whatever the preview says — a search that found a row only to hide it found nothing.
  let query = $state('');
  const needle = $derived(query.trim().toLowerCase());
  const matches = (row: { title: string; slug: string }) =>
    needle === '' || row.title.toLowerCase().includes(needle) || row.slug.includes(needle);
  // A stack is wider than a single mark, so a section holding any stack gives every
  // row's leading slot the stack's width — otherwise the titles beside a mark and
  // beside a stack start at different places in one card.
  const hasStack = (rows: PageData['sections'][number]['rows']) =>
    rows.some((row) => row.companies.length > 0 || row.flags.length > 1);
  const visibleSections = $derived(
    data.sections.filter((section) => section.rows.some(matches)),
  );
</script>

<Seo
  title="Collections · freehire"
  description="Curated collections of open tech jobs — remote by region, languages, frameworks, seniority, Y Combinator–backed companies, Big Tech, and more."
  {canonical}
/>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">Collections</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Curated groups of roles and companies, with their open jobs in one feed.
    </p>
  </header>

  <div class="relative mb-6 max-w-md">
    <Search class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
    <input
      type="search"
      placeholder="Search collections — Python, Remote Europe, Y Combinator…"
      bind:value={query}
      aria-label="Search collections"
      class="w-full rounded-lg border border-border bg-background py-2 pr-3 pl-9 text-sm transition outline-none focus:border-brand focus:ring-2 focus:ring-brand-ring/40"
    />
  </div>

  {#if visibleSections.length === 0}
    <p class="py-8 text-center text-sm text-muted-foreground">No collection matches “{query.trim()}”.</p>
  {/if}

  <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
    {#each visibleSections as section (section.key)}
      {@const wide = hasStack(section.rows)}
      <section class="flex flex-col rounded-xl border border-border bg-background p-6">
        <h2 class="text-lg font-semibold tracking-tight">
          <a href={resolve('/collections/group/[group]', { group: section.slug })} class="hover:underline">
            {section.title}
          </a>
        </h2>
        <ul class="mt-4 mb-4 space-y-1">
          {#each section.rows as row, i (row.slug)}
            <li class:hidden={!matches(row) || (needle === '' && i >= PREVIEW)}>
              <a
                href={resolve('/collections/[slug]', { slug: row.slug })}
                class="-mx-2 flex h-12 items-center gap-3 rounded-lg px-2 transition-colors hover:bg-secondary/60"
              >
                <span class={['flex shrink-0', wide && 'w-20']}>
                  <CollectionRowMark {row} />
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium text-foreground">{row.title}</span>
                  {#if row.count !== null}
                    <span class="block font-mono text-xs text-muted-foreground">
                      {row.count.toLocaleString()} jobs
                    </span>
                  {/if}
                </span>
              </a>
            </li>
          {/each}
        </ul>
        {#if needle === ''}
          <a
            href={resolve('/collections/group/[group]', { group: section.slug })}
            class="mt-auto inline-flex items-center gap-1.5 self-start text-xs font-semibold tracking-wide text-foreground uppercase hover:text-muted-foreground"
          >
            View all {section.rows.length}
            <ArrowRight class="size-3.5" />
          </a>
        {/if}
      </section>
    {/each}
  </div>
</div>
