<script lang="ts">
  import type { Company } from '$lib/types';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFollowButton from './CompanyFollowButton.svelte';

  // The company's header card: identity (logo + name + tagline + follow CTA) with the
  // industries and website link below a divider. The scalar company facts live in a
  // separate CompanyFacts card (jobs sidebar on desktop, under the header on mobile),
  // so this stays compact. Present-only: with no tagline/industries/website the card
  // is dropped and only the plain, borderless identity row remains (unenriched
  // companies unchanged).
  let { company, slug }: { company: Company; slug: string } = $props();

  const info = $derived(company.company_info ?? {});
  const industries = $derived(company.industries ?? []);
  const website = $derived(info.homepage);
  const websiteHref = $derived(website ? (website.startsWith('http') ? website : `https://${website}`) : '');

  const hasMeta = $derived(industries.length > 0 || !!website);
  const hasInfo = $derived(!!company.tagline || hasMeta);
</script>

{#snippet identity()}
  <div class="flex items-start gap-3">
    <CompanyLogo name={company.name} size="size-11" />
    <div class="min-w-0">
      <h1 class="text-2xl font-semibold tracking-tight">{company.name}</h1>
      {#if company.tagline}
        <p class="mt-1 text-sm text-muted-foreground">{company.tagline}</p>
      {/if}
    </div>
    <div class="ml-auto">
      <CompanyFollowButton {slug} companyName={company.name} />
    </div>
  </div>
{/snippet}

{#if hasInfo}
  <section class="rounded-2xl border border-border bg-card p-5">
    {@render identity()}
    {#if hasMeta}
      <div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-4">
        {#each industries as industry (industry)}
          <span class="rounded-full bg-secondary px-2.5 py-0.5 text-xs text-secondary-foreground">{industry}</span>
        {/each}
        {#if website}
          <a
            class="text-sm font-medium text-primary hover:underline"
            href={websiteHref}
            target="_blank"
            rel="noopener noreferrer">{website} ↗</a
          >
        {/if}
      </div>
    {/if}
  </section>
{:else}
  {@render identity()}
{/if}
