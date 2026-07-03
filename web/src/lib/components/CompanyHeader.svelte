<script lang="ts">
  import type { Company } from '$lib/types';
  import { countryLabel } from '$lib/facets';
  import CompanyLogo from './CompanyLogo.svelte';
  import CompanyFollowButton from './CompanyFollowButton.svelte';

  // The company's header: identity (logo + name + tagline + follow CTA) fused with
  // the authoritative company-info facts into one cohesive card. Every info field is
  // optional (populated by the backfill), so the card renders "present-only": the
  // facts panel and industries block appear only when they have content, and a company
  // with no info at all falls back to a plain, borderless identity row (unchanged).
  let { company, slug }: { company: Company; slug: string } = $props();

  const info = $derived(company.company_info ?? {});

  // Compact money label: $250M, $1.2B, $500K.
  function formatAmount(n: number): string {
    if (n >= 1_000_000_000) return `$${(n / 1_000_000_000).toFixed(n % 1_000_000_000 ? 1 : 0)}B`;
    if (n >= 1_000_000) return `$${(n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0)}M`;
    if (n >= 1_000) return `$${Math.round(n / 1_000)}K`;
    return `$${n}`;
  }

  const fundingLine = $derived(
    info.funding
      ? [info.funding.type, info.funding.amount ? formatAmount(info.funding.amount) : null, info.funding.year]
          .filter(Boolean)
          .join(' · ')
      : ''
  );
  // "NASDAQ: ACME", or just "ACME" when the exchange is unknown.
  const stockLine = $derived(
    info.stock?.symbol ? [info.stock.exchange, info.stock.symbol].filter(Boolean).join(': ') : ''
  );

  // The scalar facts, as ordered {term, value} pairs — present-only, so an absent
  // field drops out of the definition list rather than showing a blank row.
  const facts = $derived(
    [
      company.year_founded ? { term: 'Founded', value: String(company.year_founded) } : null,
      company.employee_count
        ? { term: 'Employees', value: company.employee_count.toLocaleString() }
        : null,
      company.hq_country ? { term: 'Headquarters', value: countryLabel(company.hq_country) } : null,
      company.organization_type ? { term: 'Type', value: company.organization_type } : null,
      stockLine ? { term: 'Listed', value: stockLine } : null,
      fundingLine ? { term: 'Funding', value: fundingLine } : null,
      info.parent ? { term: 'Parent', value: info.parent } : null,
      info.subsidiaries?.length ? { term: 'Subsidiaries', value: info.subsidiaries.join(', ') } : null,
    ].filter((f): f is { term: string; value: string } => !!f)
  );

  const industries = $derived(company.industries ?? []);
  const website = $derived(info.homepage);
  const websiteHref = $derived(website ? (website.startsWith('http') ? website : `https://${website}`) : '');

  // Which sides of the split have content — drives the layout and the graceful
  // collapse to a single column (or, with no info at all, a bare identity row).
  const leftHasContent = $derived(industries.length > 0 || !!website);
  const rightHasContent = $derived(facts.length > 0);
  const hasBody = $derived(leftHasContent || rightHasContent);
  const hasInfo = $derived(!!company.tagline || hasBody);
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

{#snippet industriesBlock()}
  {#if industries.length}
    <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Industries</p>
    <div class="mt-2.5 flex flex-wrap gap-1.5">
      {#each industries as industry (industry)}
        <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">{industry}</span>
      {/each}
    </div>
  {/if}
  {#if website}
    <a
      class="inline-block text-sm font-medium text-primary hover:underline {industries.length ? 'mt-3' : ''}"
      href={websiteHref}
      target="_blank"
      rel="noopener noreferrer">{website} ↗</a
    >
  {/if}
{/snippet}

{#snippet factsBlock()}
  <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Company facts</p>
  <dl class="mt-2.5 grid grid-cols-[auto_1fr] items-baseline gap-x-4 gap-y-2 text-sm">
    {#each facts as fact (fact.term)}
      <dt class="text-muted-foreground">{fact.term}</dt>
      <dd class="text-right font-semibold">{fact.value}</dd>
    {/each}
  </dl>
{/snippet}

{#if hasInfo}
  <section class="overflow-hidden rounded-2xl border border-border bg-card">
    <div class="p-5">{@render identity()}</div>

    {#if hasBody}
      {#if leftHasContent && rightHasContent}
        <div class="grid border-t border-border sm:grid-cols-[1fr_0.82fr]">
          <div class="p-5">{@render industriesBlock()}</div>
          <div class="border-t border-border bg-muted p-5 sm:border-l sm:border-t-0">
            {@render factsBlock()}
          </div>
        </div>
      {:else}
        <div class="border-t border-border p-5">
          {#if rightHasContent}{@render factsBlock()}{:else}{@render industriesBlock()}{/if}
        </div>
      {/if}
    {/if}
  </section>
{:else}
  {@render identity()}
{/if}
