// First-touch attribution: where the visit that ends in a sign-up came from.
//
// Google Analytics only sees visitors who accepted analytics cookies, so it undercounts
// sign-ups by roughly two thirds. This file records the visit's own context (campaign
// tags, the referring HOST, the landing path) and hands it to the server once, right
// after the account is created, so the database holds the complete count.
//
// It is kept in sessionStorage: it survives the full-page redirect out to an OAuth
// provider and back (the case that matters), lives no longer than the tab, and sets no
// cookie. It never holds a full URL — a query string can carry a token or an address.

const STORAGE_KEY = 'fh:first-touch';

/** The slice of Storage this file needs, so a test can hand it a map. */
export type TouchStore = Pick<Storage, 'getItem' | 'setItem'>;

export interface FirstTouch {
  utm_source?: string;
  utm_medium?: string;
  utm_campaign?: string;
  utm_content?: string;
  utm_term?: string;
  referrer_host?: string;
  landing_path?: string;
}

const UTM_KEYS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term'] as const;

function hostOf(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase();
  } catch {
    return '';
  }
}

/** Remember the first touch of this tab. A later page of the same visit never
 *  overwrites it, and neither does the redirect back from an OAuth provider.
 *
 *  `isHandoff` marks referrers that are a step of signing in or paying rather than a
 *  source (accounts.google.com, …); `ownHost` is this site, whose own pages are not a
 *  source either. */
export function captureFirstTouch(
  input: { href: string; referrer: string },
  store: TouchStore,
  ownHost: string,
  isHandoff: (referrer: string) => boolean,
): void {
  try {
    if (store.getItem(STORAGE_KEY) !== null) return;
    const url = new URL(input.href);
    const touch: FirstTouch = { landing_path: url.pathname };
    for (const key of UTM_KEYS) {
      const value = url.searchParams.get(key);
      if (value) touch[key] = value;
    }
    const host = hostOf(input.referrer);
    if (host && host !== ownHost && !isHandoff(input.referrer)) touch.referrer_host = host;
    store.setItem(STORAGE_KEY, JSON.stringify(touch));
  } catch {
    // A browser that refuses storage simply reports nothing.
  }
}

/** The remembered first touch, or an empty one. */
export function readFirstTouch(store: TouchStore): FirstTouch {
  try {
    const raw = store.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as FirstTouch) : {};
  } catch {
    return {};
  }
}

/** Google Analytics' client id from the `_ga` cookie ("GA1.1.123.456" → "123.456").
 *  The cookie exists only for a visitor who accepted analytics, which is what makes
 *  it safe to hand to the server for the GA4 sign-up. */
export function gaClientId(cookie: string): string | undefined {
  const match = /(?:^|;\s*)_ga=GA\d+\.\d+\.(\d+\.\d+)/.exec(cookie);
  return match?.[1];
}

/** The current GA session id from the `_ga_<container>` cookie, in either the GS1
 *  ("GS1.1.1700000000.…") or GS2 ("GS2.1.s1700000000$o1…") layout. */
export function gaSessionId(cookie: string, measurementId: string): string | undefined {
  const name = `_ga_${measurementId.replace(/^G-/, '')}`;
  const match = new RegExp(`(?:^|;\\s*)${name}=GS\\d\\.\\d\\.(?:s)?(\\d+)`).exec(cookie);
  return match?.[1];
}

/** Tell the server how this just-created account arrived. Best effort: attribution
 *  must never be able to disturb a sign-up, so no failure is thrown.
 *
 *  Resolves true when the browser must NOT send the GA4 `sign_up` itself, false when
 *  it must. GA4 cannot dedupe, so a doubtful outcome counts as "the server has it":
 *  the server confirmed it, or the request went out and no answer could be read (it
 *  may well have been processed). Only a definite failure - the request could not be
 *  built, or the server refused it - returns false. */
export async function reportSignupAttribution(method: 'oauth' | 'password', measurementId: string): Promise<boolean> {
  let init: RequestInit;
  try {
    const cookie = document.cookie;
    init = {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...readFirstTouch(sessionStorage),
        method,
        ga_client_id: gaClientId(cookie),
        ga_session_id: gaSessionId(cookie, measurementId),
      }),
      keepalive: true,
    };
  } catch {
    return false; // nothing was sent
  }
  try {
    const response = await fetch('/api/v1/me/attribution', init);
    if (!response.ok) return false;
    const answer = (await response.json()) as { data?: { ga4_sent?: boolean } };
    return answer.data?.ga4_sent === true;
  } catch {
    return true; // sent, but no readable answer: the server may own the event
  }
}
