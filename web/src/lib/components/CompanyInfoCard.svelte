<script lang="ts">
  import type { Company } from '$lib/types';
  import { countryLabel } from '$lib/facets';

  // The company's authoritative company-info facts, rendered above the jobs list.
  // Every field is optional (populated by the backfill); the whole card renders
  // nothing when the company has no info, so unenriched companies are unchanged.
  let { company }: { company: Company } = $props();

  const info = $derived(company.company_info ?? {});

  // Inline facts, present-only, in a stable order.
  const facts = $derived(
    [
      company.year_founded ? `Founded ${company.year_founded}` : null,
      company.employee_count ? `${company.employee_count.toLocaleString()} employees` : null,
      company.hq_country ? countryLabel(company.hq_country) : null,
      company.organization_type || null,
    ].filter((f): f is string => !!f)
  );

  const industries = $derived(company.industries ?? []);
  const website = $derived(info.homepage);
  const subsidiaries = $derived(info.subsidiaries ?? []);

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
  const websiteHref = $derived(website ? (website.startsWith('http') ? website : `https://${website}`) : '');

  const hasInfo = $derived(
    !!company.tagline ||
      facts.length > 0 ||
      industries.length > 0 ||
      !!website ||
      !!fundingLine ||
      !!stockLine ||
      !!info.parent ||
      subsidiaries.length > 0
  );
</script>

{#if hasInfo}
  <section class="mt-4 rounded-xl border border-border bg-card p-4 text-sm">
    {#if company.tagline}
      <p class="text-muted-foreground">{company.tagline}</p>
    {/if}

    {#if facts.length}
      <p class="mt-2 font-medium">{facts.join(' · ')}</p>
    {/if}

    {#if industries.length}
      <div class="mt-3 flex flex-wrap gap-1.5">
        {#each industries as industry (industry)}
          <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">{industry}</span>
        {/each}
      </div>
    {/if}

    {#if fundingLine || stockLine || info.parent || subsidiaries.length}
      <dl class="mt-3 grid gap-1 text-xs text-muted-foreground">
        {#if fundingLine}
          <div><dt class="inline font-medium text-foreground">Funding:</dt> {fundingLine}</div>
        {/if}
        {#if stockLine}
          <div><dt class="inline font-medium text-foreground">Listed:</dt> {stockLine}</div>
        {/if}
        {#if info.parent}
          <div><dt class="inline font-medium text-foreground">Parent:</dt> {info.parent}</div>
        {/if}
        {#if subsidiaries.length}
          <div><dt class="inline font-medium text-foreground">Subsidiaries:</dt> {subsidiaries.join(', ')}</div>
        {/if}
      </dl>
    {/if}

    {#if website}
      <a
        class="mt-3 inline-block text-primary hover:underline"
        href={websiteHref}
        target="_blank"
        rel="noopener noreferrer">{website} ↗</a
      >
    {/if}
  </section>
{/if}
