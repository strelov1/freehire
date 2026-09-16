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

/** The proxy logo URL for a SOURCE, resolved from the host its own postings live on —
 *  `greenhouse.io` for Greenhouse, an employer's own domain for a single-company career
 *  page. Same proxy, same 404-into-a-placeholder behaviour as `companyLogoUrl`.
 *
 *  The host comes from a stored posting rather than a hand-kept map of domains, so a
 *  source with no postings has no host and gets no logo. Null here is that case, and the
 *  caller's placeholder is the right answer to it — a map would have gone stale silently
 *  and the entry it was missing would have been invisible. */
export function sourceLogoUrl(host: string | null): string | null {
  if (!host) return null;
  return `${COMPANY_LOGO_BASE}/${encodeURIComponent(host)}`;
}
