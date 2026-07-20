// Company logos are served from our own logo.freehire.dev proxy. It fronts a free
// favicon service (faviconapi.com) that resolves the real brand mark for most
// company names; on a miss the proxy returns a clean 404 so each consumer falls
// back to its own placeholder (the SVG monogram in the SPA, a monogram tile in the
// OG card). No third-party token or attribution ships in the browser — the proxy
// owns everything server-side. Resolves by company name, since that is all most
// call sites carry (job rows, search, referrals).
export const COMPANY_LOGO_BASE = 'https://logo.freehire.dev';

/** The proxy logo URL for a company name, or null when there is no name. */
export function companyLogoUrl(name: string): string | null {
  if (!name) return null;
  return `${COMPANY_LOGO_BASE}/${encodeURIComponent(name)}`;
}
