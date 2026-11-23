// Cookie-consent pure logic: region classification and consent persistence, kept
// free of Svelte runes and SvelteKit imports so it unit-tests in the plain-node
// vitest environment. The reactive wrapper (grant/deny/reopen over $state) lives
// in consent.svelte.ts; this module holds only pure functions and localStorage IO.

/** The visitor's recorded cookie choice; absence means no choice made yet. */
export type ConsentChoice = 'granted' | 'denied';

/** localStorage key holding the consent choice. */
export const CONSENT_KEY = 'hire.consent';

// EEA/EU territories that sit outside the `Europe/*` timezone prefix. Kept explicit
// and small; erring toward inclusion only ever shows the banner to a few extra
// visitors, which is harmless.
const EEA_ATLANTIC_ZONES = new Set([
  'Atlantic/Canary', // Spain
  'Atlantic/Madeira', // Portugal
  'Atlantic/Azores', // Portugal
  'Atlantic/Reykjavik', // Iceland (EEA)
  'Atlantic/Faroe', // Denmark
]);

/** True when a browser timezone places the visitor in a consent-required region
 *  (EU/EEA/UK). Unknown or empty input fails toward asking (returns true), so a
 *  visitor we cannot classify is treated conservatively. */
export function regionNeedsConsent(timezone: string): boolean {
  if (!timezone) return true;
  if (timezone.startsWith('Europe/')) return true;
  if (EEA_ATLANTIC_ZONES.has(timezone)) return true;
  // A valid IANA zone that is neither European nor an EEA Atlantic one is not
  // consent-required; anything Intl rejects as unknown fails toward asking.
  try {
    Intl.DateTimeFormat(undefined, { timeZone: timezone });
    return false;
  } catch {
    return true;
  }
}

/** True when the banner should be shown: a consent-required visitor who has not
 *  yet made a choice. Any recorded choice (or a non-required region) suppresses it. */
export function needsBanner(regionRequired: boolean, choice: ConsentChoice | null): boolean {
  return regionRequired && choice === null;
}

/** Read the stored choice, or null when none is recorded, the value is
 *  unrecognized, or storage is unavailable (SSR). Never throws. */
export function readConsent(): ConsentChoice | null {
  try {
    const value = localStorage.getItem(CONSENT_KEY);
    return value === 'granted' || value === 'denied' ? value : null;
  } catch {
    return null;
  }
}

/** Persist the visitor's choice. A no-op when storage is unavailable. Never throws. */
export function writeConsent(choice: ConsentChoice): void {
  try {
    localStorage.setItem(CONSENT_KEY, choice);
  } catch {
    // Storage unavailable or full (SSR, private mode) — consent simply won't persist.
  }
}

/** `document.cookie` assignments that expire Google Analytics' `_ga` and `_ga_<container>`
 *  cookies. gtag sets them on the widest domain it may (`.freehire.me`), so each is expired
 *  for every parent domain of the host as well as host-only; a domain the browser refuses
 *  (a public suffix) is ignored by it. Withdrawing consent must remove them: the server
 *  reads the client id from them to send the GA4 sign-up. */
export function gaCookieExpiries(cookie: string, hostname: string): string[] {
  const names = cookie
    .split(';')
    .map((part) => (part.split('=')[0] ?? '').trim())
    .filter((name) => name === '_ga' || name.startsWith('_ga_'));
  const labels = hostname.split('.');
  const domains = labels.slice(0, -1).map((_, i) => `; domain=${labels.slice(i).join('.')}`);
  return names.flatMap((name) => ['', ...domains].map((domain) => `${name}=; Max-Age=0; path=/${domain}`));
}

/** Expire the GA cookies in this browser. A no-op without a document (SSR). Never throws. */
export function clearGaCookies(): void {
  try {
    for (const expiry of gaCookieExpiries(document.cookie, location.hostname)) document.cookie = expiry;
  } catch {
    // No document (SSR) — nothing to clear.
  }
}
