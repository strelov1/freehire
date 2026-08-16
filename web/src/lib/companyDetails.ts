// The "what do we actually know about this company" derivations, lifted out of the
// components that render them. Three surfaces ask the same question — the facts card,
// the About card, and the job page's Company tab, which must decide whether to show
// its panel or say it holds nothing — and a second copy of these conditions would
// drift: the tab would promise details while both cards rendered nothing.
//
// Kept free of runes and `$app/*` so it runs under the repo's node-environment vitest
// (web/vitest.config.ts), the same shape as companyFacetModel.ts beside it.
import { countryLabel } from './facets';
import type { Company } from './types';

/** One row of the facts card: a term, its display value, and — Headquarters only —
 *  the ISO country code whose flag precedes the value. */
export type CompanyFact = { term: string; value: string; flag?: string };

/** The company's full summary, or '' when it holds none. */
export function companyDescription(company: Company): string {
  return company.company_info?.description?.trim() ?? '';
}

/** The curated YC-directory flags, as display labels. */
export function companyBadges(company: Company): string[] {
  const info = company.company_info ?? {};
  return [
    info.top_company ? 'YC Top Company' : null,
    info.is_hiring ? 'Hiring' : null,
    info.stage ? `${info.stage}-stage` : null,
  ].filter((b): b is string => !!b);
}

/** Compact money label: $250M, $1.2B, $500K. */
function formatAmount(n: number): string {
  if (n >= 1_000_000_000) return `$${(n / 1_000_000_000).toFixed(n % 1_000_000_000 ? 1 : 0)}B`;
  if (n >= 1_000_000) return `$${(n / 1_000_000).toFixed(n % 1_000_000 ? 1 : 0)}M`;
  if (n >= 1_000) return `$${Math.round(n / 1_000)}K`;
  return `$${n}`;
}

/** The company's scalar facts, in display order. Present-only: an absent field drops
 *  out of the list rather than becoming a blank row. */
export function companyFacts(company: Company): CompanyFact[] {
  const info = company.company_info ?? {};

  const funding = info.funding
    ? [info.funding.type, info.funding.amount ? formatAmount(info.funding.amount) : null, info.funding.year]
        .filter(Boolean)
        .join(' · ')
    : '';
  // "NASDAQ: ACME", or just "ACME" when the exchange is unknown.
  const stock = info.stock?.symbol
    ? [info.stock.exchange, info.stock.symbol].filter(Boolean).join(': ')
    : '';

  return [
    company.year_founded ? { term: 'Founded', value: String(company.year_founded) } : null,
    company.employee_count
      ? { term: 'Employees', value: company.employee_count.toLocaleString() }
      : null,
    company.hq_country
      ? { term: 'Headquarters', value: countryLabel(company.hq_country), flag: company.hq_country }
      : null,
    company.organization_type ? { term: 'Type', value: company.organization_type } : null,
    stock ? { term: 'Listed', value: stock } : null,
    funding ? { term: 'Funding', value: funding } : null,
    info.parent ? { term: 'Parent', value: info.parent } : null,
    info.subsidiaries?.length ? { term: 'Subsidiaries', value: info.subsidiaries.join(', ') } : null,
  ].filter((f): f is CompanyFact => !!f);
}

/** Whether the company has anything at all worth rendering. False means every card
 *  below would render nothing, so a caller must not put a heading above them. */
export function hasCompanyDetails(company: Company): boolean {
  return (
    companyFacts(company).length > 0 ||
    companyBadges(company).length > 0 ||
    companyDescription(company) !== ''
  );
}
