<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import Seo from '$lib/components/Seo.svelte';
  import { breadcrumbJsonLd, jsonLdScript } from '$lib/seo';
  import { Card } from '$lib/ui';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const origin = $derived(page.url.origin);
  const canonical = $derived(`${origin}/roles`);
  const count = (n: number) => n.toLocaleString('en-US');
  const description = $derived(
    `Open jobs by role and country, across ${data.categories.length} roles and ${count(data.total)} live postings.`,
  );

  const jsonLd = $derived(
    jsonLdScript([
      breadcrumbJsonLd([
        { name: 'freehire', url: `${origin}/` },
        { name: 'Roles', url: canonical },
      ]),
    ]),
  );
</script>

<Seo title="Jobs by Role and Country · freehire" {description} {canonical} />
<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD from jsonLdScript, which escapes `<` -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-5xl px-4 py-6">
  <header class="mb-8">
    <h1 class="text-2xl font-semibold tracking-tight">Jobs by Role and Country</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>
  </header>

  <ul class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
    {#each data.categories as c (c.category)}
      <li>
        <a href={resolve('/roles/[category]', { category: c.slug })} class="block">
          <Card class="p-4 transition-colors hover:bg-accent">
            <span class="font-medium">{c.label}</span>
            <span class="block text-sm text-muted-foreground">{count(c.openCount)} open jobs</span>
          </Card>
        </a>
      </li>
    {/each}
  </ul>
</div>
