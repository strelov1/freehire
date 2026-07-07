<script lang="ts">
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  // Self-canonical: the landing page is the collection's canonical home, never the
  // bare /jobs the raw ?collections= filter would resolve to.
  const canonical = $derived(`${origin}/collections/${data.slug}`);
  // Template copy from the collection's display title (see design): "<title> jobs".
  const heading = $derived(`${data.collection.title} jobs`);
  // Request hiding the facets the collection pins via `scope`. Note: this only
  // hides standalone facets; the collection facets that live in composite panes
  // (collections/work_mode/regions) stay visible in the filter UI for now, but
  // `scope` re-pins them on every search so the user can never actually remove the
  // collection's constraint (JobsView.scopedParams) — hiding those controls is a
  // deferred UI polish, not a correctness gap.
  const excludeFacets = $derived(Object.keys(data.collection.params));
  // Breadcrumb structured data (freehire → Collections → this collection), mirroring
  // the company landing — this is an SEO landing page, so the trail helps engines.
  const jsonLd = $derived(
    jsonLdScript(
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Collections', url: `${origin}/collections` },
        { name: heading, url: canonical },
      ])
    )
  );
</script>

<Seo title={`${heading} · freehire`} description={data.collection.description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">{heading}</h1>
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
