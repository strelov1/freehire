<script lang="ts">
  import { page } from '$app/state';
  import CompanyView from '$lib/components/CompanyView.svelte';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript, organizationJsonLd } from '$lib/seo';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/companies/${data.slug}`);
  const description = $derived(`Open jobs at ${data.company.name}, aggregated by freehire.`);
  const jsonLd = $derived(
    jsonLdScript([
      organizationJsonLd(data.company, origin),
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Companies', url: `${origin}/companies` },
        { name: data.company.name, url: canonical },
      ]),
    ])
  );
</script>

<Seo title={`${data.company.name} · freehire`} {description} {canonical} />

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <!-- Remount on slug change so the seeded paginator/filters start fresh per company. -->
  {#key data.slug}
    <CompanyView company={data.company} initial={data.initial} slug={data.slug} />
  {/key}
</div>
