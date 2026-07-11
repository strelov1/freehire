<script lang="ts">
  import type { Company } from '$lib/types';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFollowButton from './CompanyFollowButton.svelte';

  // The company's header card: identity (logo + name + tagline + follow CTA) with the
  // industries and the website/LinkedIn links below a divider. The scalar company facts
  // live in a separate CompanyFacts card (jobs sidebar on desktop, under the header on
  // mobile), so this stays compact. The card is always shown, even for an unenriched
  // company (just the identity row) — the tagline and the meta divider are present-only.
  let { company, slug }: { company: Company; slug: string } = $props();

  const info = $derived(company.company_info ?? {});
  const industries = $derived(company.industries ?? []);
  // The homepage lives under `website` (hirebase) or `homepage` (older records) — read
  // whichever is present so the link renders regardless of which importer filled it.
  const website = $derived(info.website ?? info.homepage);
  const websiteHref = $derived(website ? (website.startsWith('http') ? website : `https://${website}`) : '');
  const linkedin = $derived(info.linkedin);

  const hasMeta = $derived(industries.length > 0 || !!website || !!linkedin);
</script>

<section class="rounded-2xl border border-border bg-card p-5">
  <div class="flex items-start gap-3">
    <CompanyLogo name={company.name} size="size-11" />
    <div class="min-w-0 flex-1">
      <h1 class="text-2xl font-semibold tracking-tight">{company.name}</h1>
      {#if company.tagline}
        <p class="mt-1 text-sm text-muted-foreground">{company.tagline}</p>
      {/if}
    </div>
    <!-- flex-1 above lets the name/tagline use the full width; the follow CTA stays
         top-right and never shrinks (it collapses to an icon on mobile itself). -->
    <div class="shrink-0">
      <CompanyFollowButton {slug} companyName={company.name} />
    </div>
  </div>
  {#if hasMeta}
    <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-3">
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
      {#if linkedin}
        <a
          class="text-sm font-medium text-primary hover:underline"
          href={linkedin}
          target="_blank"
          rel="noopener noreferrer">LinkedIn ↗</a
        >
      {/if}
    </div>
  {/if}
</section>
