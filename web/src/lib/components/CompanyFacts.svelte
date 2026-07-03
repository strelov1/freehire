<script lang="ts">
  import type { Company } from '$lib/types';
  import { countryLabel } from '$lib/facets';

  // The company's scalar facts as a self-contained card, shown in the jobs sidebar
  // (desktop) and as a fallback card under the header (mobile, where the sidebar is
  // hidden). Present-only: renders nothing when the company has no facts, so the
  // wrapper never leaves an empty box.
  let { company }: { company: Company } = $props();

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

  // Ordered {term, value} pairs — present-only, so an absent field drops out of the
  // definition list rather than showing a blank row.
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
</script>

{#if facts.length}
  <div class="rounded-xl border border-border bg-card p-4">
    <p class="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Company facts</p>
    <dl class="grid grid-cols-[auto_1fr] items-baseline gap-x-3 gap-y-2 text-sm">
      {#each facts as fact (fact.term)}
        <dt class="text-muted-foreground">{fact.term}</dt>
        <dd class="text-right font-medium">{fact.value}</dd>
      {/each}
    </dl>
  </div>
{/if}
