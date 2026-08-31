<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import {
    breadcrumbJsonLd,
    collectionHeading,
    collectionPageJsonLd,
    jobListItems,
    jsonLdScript,
  } from '$lib/seo';
  import { backerBadges } from '$lib/backers';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const base = $derived(`${origin}/collections/${data.slug}`);
  // Self-canonical: the landing page is the collection's canonical home, never the
  // bare /jobs the raw ?collections= filter would resolve to. Each paginated page
  // is canonical to ITSELF, not to page one — pointing them all at the first page
  // tells Google the deeper rows are duplicates of it, which is how the rows those
  // pages exist to expose stop being indexed.
  const canonical = $derived(data.pageNumber > 1 ? `${base}?page=${data.pageNumber}` : base);
  // Template copy from the collection's display title, with its live open-job
  // count (see design): "<total> <title> jobs".
  const heading = $derived(collectionHeading(data.collection.title, data.initial.total));
  // The backing brand's mark, where this collection names one. A filter collection
  // or an editorial theme has none, and then the heading stands alone.
  const mark = $derived(backerBadges([data.slug])[0]?.mark ?? null);
  // Page 2 onward says so in the title: without it every page of a collection
  // competes for the same SERP entry under one identical title.
  const pageTitle = $derived(
    data.pageNumber > 1 ? `${heading} — page ${data.pageNumber} · freehire` : `${heading} · freehire`,
  );
  // Request hiding the facets the collection pins via `scope`. Note: this only
  // hides standalone facets; the collection facets that live in composite panes
  // (collections/work_mode/regions) stay visible in the filter UI for now, but
  // `scope` re-pins them on every search so the user can never actually remove the
  // collection's constraint (JobsView.scopedParams) — hiding those controls is a
  // deferred UI polish, not a correctness gap.
  const excludeFacets = $derived(Object.keys(data.collection.params));
  // Structured data for this SEO landing page: a CollectionPage wrapping the
  // first page of jobs as an ItemList (so engines read the page as a curated
  // collection), plus a breadcrumb trail (freehire → Collections → this one),
  // mirroring the company landing.
  const jsonLd = $derived(
    jsonLdScript([
      collectionPageJsonLd(
        heading,
        data.collection.description,
        canonical,
        jobListItems(data.initial.items, origin)
      ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Collections', url: `${origin}/collections` },
        { name: heading, url: base },
      ]),
    ])
  );
</script>

<Seo title={pageTitle} description={data.collection.description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <header class="mb-8">
    <h1 class="flex items-center gap-2.5 text-2xl font-semibold tracking-tight">
      <!-- Decorative: the heading right beside it names the same brand, so a screen
           reader would only hear it twice. -->
      {#if mark}
        <img src={mark} alt="" class="size-7 shrink-0 rounded-md object-contain" />
      {/if}
      {heading}
    </h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      {data.collection.description}
    </p>
    {#if data.marketLink}
      <p class="mt-3 text-sm">
        <a
          href={resolve('/roles/[category]', { category: data.marketLink.slug })}
          class="text-brand-strong hover:underline"
        >
          {data.marketLink.label} jobs by country — openings, pay and top skills →
        </a>
      </p>
    {/if}
  </header>

  <!-- Remount on slug change so the seeded paginator/filters start fresh per
       collection (mirrors the company page). -->
  {#key data.slug}
    <JobsView
      initial={data.initial}
      scope={data.collection.params}
      {excludeFacets}
      currentPage={data.pageNumber}
    />
  {/key}
</div>
