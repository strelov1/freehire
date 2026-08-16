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

/** One outbound link, ready for an icon. `key` picks the mark, `label` names the link
 *  for a screen reader, since the icon alone carries no accessible text. */
export type CompanySocial = {
  key: 'website' | 'linkedin' | 'twitter' | 'facebook' | 'instagram';
  label: string;
  href: string;
};

// Fixed display order. Site first — it is the one most people want and the one most
// companies have — then the networks by how often they are filled in.
const SOCIAL_ORDER = [
  ['website', 'website'],
  ['linkedin', 'on LinkedIn'],
  ['twitter', 'on X'],
  ['facebook', 'on Facebook'],
  ['instagram', 'on Instagram'],
] as const;

/** Whether a stored link may be put in an href.
 *
 *  `company_info` is written by an external importer, not by us, and these values go
 *  straight into an anchor. `javascript:` and `data:` there execute script on our own
 *  origin, so the scheme is allow-listed rather than cleaned up: anything that is not
 *  plain http(s) is dropped entirely. A protocol-relative `//host` is refused too —
 *  `URL` cannot parse it without a base, which is exactly the ambiguity we don't want. */
function isSafeUrl(raw: string): boolean {
  try {
    const { protocol } = new URL(raw);
    return protocol === 'http:' || protocol === 'https:';
  } catch {
    return false;
  }
}

/** The company's outbound links, in display order. Present-only, and silently short of
 *  any link whose scheme we refuse. */
export function companySocials(company: Company): CompanySocial[] {
  const info = company.company_info ?? {};
  const out: CompanySocial[] = [];
  for (const [key, suffix] of SOCIAL_ORDER) {
    const href = info[key]?.trim();
    if (!href || !isSafeUrl(href)) continue;
    out.push({ key, label: `${company.name} ${suffix}`, href });
  }
  return out;
}

/** The countries the company has an office in, as upper-cased ISO 3166-1 alpha-2 codes,
 *  deduplicated in first-seen order. Anything that is not a two-letter code is dropped:
 *  the importer's `locations` is free-form enough to carry country names too, and a
 *  flag component given "USA" would render nothing useful. */
export function companyLocations(company: Company): string[] {
  const seen = new Set<string>();
  for (const loc of company.company_info?.locations ?? []) {
    const code = loc.code?.trim().toUpperCase();
    if (!code || !/^[A-Z]{2}$/.test(code)) continue;
    seen.add(code);
  }
  return [...seen];
}

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
    info.ceo ? { term: 'CEO', value: info.ceo } : null,
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
    companySocials(company).length > 0 ||
    companyLocations(company).length > 0 ||
    companyDescription(company) !== ''
  );
}
