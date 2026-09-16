// Company logos are served from our own logo.freehire.me proxy. It fronts a free
// favicon service (faviconapi.com) that resolves the real brand mark for most
// company names; on a miss the proxy returns a clean 404 so each consumer falls
// back to its own placeholder (the SVG monogram in the SPA, a monogram tile in the
// OG card). No third-party token or attribution ships in the browser — the proxy
// owns everything server-side. Resolves by company name, since that is all most
// call sites carry (job rows, search, referrals).
const COMPANY_LOGO_BASE = 'https://logo.freehire.me';

/** The proxy logo URL for a company name, or null when there is no name. */
export function companyLogoUrl(name: string): string | null {
  if (!name) return null;
  return `${COMPANY_LOGO_BASE}/${encodeURIComponent(name)}`;
}

/** The proxy logo URL for a SOURCE, resolved from its DISPLAY NAME — "Greenhouse",
 *  "BambooHR", "Telegram" — exactly as `companyLogoUrl` resolves a company.
 *
 *  Not from a host, which is what the first version of this did. The proxy resolves a
 *  brand from a name; a host is either a 404 (a per-tenant subdomain such as
 *  `jobs.smartrecruiters.com` or `2020companies.wd1.myworkdayjobs.com`) or, worse, the
 *  right image for the WRONG company, because an ATS posting's URL is often on the
 *  employer's own domain — production served Bankrate's mark for Greenhouse and ZEREN
 *  GROUP's for SuccessFactors. Measured against the live proxy on 2026-09-16: 14 of 15
 *  source names resolve; hosts were about half 404 and half wrong brand.
 *
 *  A miss still 404s, so every caller needs its own fallback — the proxy cannot tell the
 *  difference between "no such brand" and "not today". */
export function sourceLogoUrl(displayName: string): string | null {
  if (!displayName) return null;
  return `${COMPANY_LOGO_BASE}/${encodeURIComponent(displayName)}`;
}
