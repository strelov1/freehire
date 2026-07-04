// Builds the HTML for a company's Open Graph card (light, 1200×630), logo-forward:
// a hero company logo, the name, its live open-jobs count, and — when present — a
// tagline and chips for the first industry and the HQ country. Pure and
// synchronous (entity + resolved logo + count → HTML string for satori), so it is
// exercised directly by the render smoke test.
//
// Shared brand primitives (mark, escaping, logo tile, chips, footer) come from
// ./shared so the job/company/brand cards cannot drift. satori constraint: layout
// is flexbox only, and any element with more than one child declares `display:flex`.

import type { Company } from '$lib/types';
import { countryLabel } from '$lib/facets';
import { OG_HEIGHT, OG_WIDTH, brandFooter, chipMarkup, esc, logoBox, type Chip } from './shared';

const LOGO_SIZE = 140;

const jobsFmt = new Intl.NumberFormat('en');

/** "1 open job" / "128 open jobs" — grouped, singular/plural correct. */
function openJobsLabel(n: number): string {
  return `${jobsFmt.format(n)} open ${n === 1 ? 'job' : 'jobs'}`;
}

/** The company chips, absent facets omitted: first industry, then HQ country name. */
function chips(company: Company): Chip[] {
  const out: Chip[] = [];
  const industry = company.industries?.[0];
  if (industry) out.push({ text: industry });
  if (company.hq_country) out.push({ text: countryLabel(company.hq_country) });
  return out;
}

/** Builds the card HTML for `company`. `logo` is a data-URI or null (monogram);
 *  `openJobs` is the company's current open-vacancy count. */
export function buildCompanyCard(
  company: Company,
  opts: { logo: string | null; openJobs: number },
): string {
  const tagline = company.tagline?.trim();
  const chipRow = chips(company)
    .map(chipMarkup)
    .join('');

  return `
<div style="display:flex;flex-direction:column;justify-content:space-between;width:${OG_WIDTH}px;height:${OG_HEIGHT}px;padding:64px 72px;background:#ffffff;color:#0a0a0a;font-family:Inter">
  <div style="display:flex;align-items:center;gap:28px">
    ${logoBox(opts.logo, company.name, LOGO_SIZE)}
    <div style="display:flex;font-size:56px;font-weight:700;letter-spacing:-0.03em;overflow:hidden">${esc(company.name)}</div>
  </div>
  <div style="display:flex;flex-direction:column;gap:18px">
    <div style="display:flex;font-size:44px;font-weight:700;letter-spacing:-0.02em">${esc(openJobsLabel(opts.openJobs))}</div>
    ${tagline ? `<div style="display:flex;font-size:28px;color:#404040;overflow:hidden">${esc(tagline)}</div>` : ''}
    ${chipRow ? `<div style="display:flex;flex-wrap:wrap;gap:12px">${chipRow}</div>` : ''}
  </div>
  ${brandFooter()}
</div>`.trim();
}
