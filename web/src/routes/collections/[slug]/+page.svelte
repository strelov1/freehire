<script lang="ts">
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, collectionPageJsonLd, jobListItems, jsonLdScript } from '$lib/seo';
  import { backerBadges } from '$lib/backers';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  // Self-canonical: the landing page is the collection's canonical home, never the
  // bare /jobs the raw ?collections= filter would resolve to.
  const canonical = $derived(`${origin}/collections/${data.slug}`);
  // Template copy from the collection's display title (see design): "<title> jobs".
  const heading = $derived(`${data.collection.title} jobs`);
  // The backing brand's mark, where this collection names one. A filter collection
  // or an editorial theme has none, and then the heading stands alone.
  const mark = $derived(backerBadges([data.slug])[0]?.mark ?? null);
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
        { name: heading, url: canonical },
      ]),
    ])
  );
</script>

<Seo title={`${heading} · freehire`} description={data.collection.description} {canonical} />

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
  </header>

  <!-- Remount on slug change so the seeded paginator/filters start fresh per
       collection (mirrors the company page). -->
  {#key data.slug}
    <JobsView initial={data.initial} scope={data.collection.params} {excludeFacets} />
  {/key}
</div>
