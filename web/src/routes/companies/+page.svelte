<script lang="ts">
  import { page } from '$app/state';
  import CompaniesView from '$lib/components/CompaniesView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import {
    breadcrumbJsonLd,
    collectionPageJsonLd,
    companyListItems,
    jsonLdScript,
  } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/companies`);
  const description =
    'Browse companies hiring in tech, with their current open roles — aggregated by freehire.';
  // Structured data for the directory: a CollectionPage wrapping the server-rendered
  // first page of companies as an ItemList, plus a breadcrumb (freehire → Companies),
  // mirroring the collection landings. The list follows the active filters, so a
  // deep-linked filtered URL describes what it actually shows.
  const jsonLd = $derived(
    jsonLdScript([
      collectionPageJsonLd(
        'Companies hiring in tech',
        description,
        canonical,
        companyListItems(data.initial.items, origin)
      ),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Companies', url: canonical },
      ]),
    ])
  );
</script>

<Seo title="Companies · freehire" {description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Primary heading kept for search engines and assistive tech, but visually
       hidden: the list and filters make the page's purpose clear, so the on-screen
       title was redundant. sr-only takes no layout space. Mirrors /jobs. -->
  <h1 class="sr-only">Companies hiring in tech</h1>
  <CompaniesView initial={data.initial} currentPage={data.pageNumber} />
</div>
