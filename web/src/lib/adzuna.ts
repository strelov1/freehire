// Adzuna's API terms require every advert we display from their feed to carry the phrase
// "Jobs by Adzuna", at least 116x23 pixels, with the word "Jobs" and the Adzuna logo each
// hyperlinked to "http://www.adzuna.co.uk or the relevant local domain".
//
// This resolves "the relevant local domain". Adzuna is one brand per country and the redirect
// URL we store already names which one a posting came from — a German posting links to
// www.adzuna.de — so the credit is read off the posting rather than configured.

/** The domain the terms name literally, used when a posting's URL cannot supply one. */
export const ADZUNA_FALLBACK_DOMAIN = 'https://www.adzuna.co.uk';

/** Every Adzuna country domain is `adzuna.<tld>`, possibly multi-part (`adzuna.com.au`). */
const ADZUNA_HOST = /(^|\.)adzuna\.[a-z]{2,}(\.[a-z]{2,})?$/;

/**
 * The local Adzuna domain to credit for a posting, from its outbound URL.
 *
 * Anything that is not an Adzuna host falls back rather than being echoed: the URL comes from a
 * stored row, and a row whose link stopped being an Adzuna one must not turn a required credit
 * into a link to somewhere else entirely.
 */
export function adzunaLocalDomain(jobUrl: string): string {
  let parsed: URL;
  try {
    parsed = new URL(jobUrl);
  } catch {
    return ADZUNA_FALLBACK_DOMAIN;
  }
  if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') return ADZUNA_FALLBACK_DOMAIN;
  if (!ADZUNA_HOST.test(parsed.hostname)) return ADZUNA_FALLBACK_DOMAIN;
  return `${parsed.protocol}//${parsed.host}`;
}
