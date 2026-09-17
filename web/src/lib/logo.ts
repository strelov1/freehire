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
export function sourceLogoUrl(displayName: string, domain?: string): string | null {
  if (!displayName) return null;
  const url = `${COMPANY_LOGO_BASE}/${encodeURIComponent(displayName)}`;
  return domain ? `${url}?domain=${encodeURIComponent(domain)}` : url;
}

/** A source's own domain, for the sources whose logo the proxy cannot find — or could find
 *  wrongly — from the name alone.
 *
 *  An OVERRIDE, not a registry: a source missing from here resolves by name exactly as it
 *  does today, so the map going stale costs a logo rather than breaking one. That is the
 *  deliberate difference from the posting-host sampling this replaced, which was wrong for
 *  every source it covered.
 *
 *  Measured against the live proxy 2026-09-17: of the 145 sources carrying 500+ jobs, these
 *  14 had no logo by name, and all 14 resolve with the domain. `successfactors` settles why
 *  a nicer name was never the whole fix — its label was already right and the proxy still
 *  had nothing. The domain also removes a doubt a 200 cannot: a name resolved WRONGLY
 *  answers 200 with somebody else's mark (see cmd/publish-logo-domains and the `g2i` case).
 *
 *  Add an entry when a source's card shows the wrong mark or none; verify it resolves before
 *  committing, the same way these were. */
export const SOURCE_LOGO_DOMAINS: Record<string, string> = {
  successfactors: 'successfactors.com',
  zohorecruit: 'zoho.com',
  jazzhr: 'jazzhr.com',
  adpmyjobs: 'adp.com',
  applicantpro: 'applicantpro.com',
  '4dayweek': '4dayweek.io',
  isolvedhire: 'isolvedhcm.com',
  hrmdirect: 'hrmdirect.com',
  freshteam: 'freshworks.com',
  catsone: 'catsone.com',
  sber: 'sber.ru',
  alfabank: 'alfabank.ru',
  aijobs: 'aijobs.net',
  remotedotcom: 'remote.com',
};
