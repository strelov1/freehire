import { browser } from 'wxt/browser';

// Product analytics for the panel, sent through the Measurement Protocol to the SAME GA4
// stream the site reports to, so one funnel reads site → extension → upgrade. Every event
// carries `surface: 'extension'`, which is what separates it from the site's own.
// (Not the stream the Chrome Web Store dashboard creates: that one is Google's, tagged by
// Google, and counts visits to the store listing — it can never see the panel.)
// The gtag.js snippet GA hands out cannot run here: MV3 forbids
// loading remote code, so events are POSTed directly instead.
//
// What is sent is deliberately thin — an event name and a few enum-like params. Never a
// page URL, page text, a message, or anything from the user's profile: those are what the
// privacy policy promises stay between the panel and freehire.me.
//
// Both values are read at build time and absent from a plain `wxt dev`, which leaves
// tracking inert so local work never lands in the production stream. The API secret is
// not a credential in the usual sense: it ships inside the bundle, and all it permits is
// sending events to this one stream.
const MEASUREMENT_ID = import.meta.env.WXT_GA_MEASUREMENT_ID;
const API_SECRET = import.meta.env.WXT_GA_API_SECRET;

const ENDPOINT = 'https://www.google-analytics.com/mp/collect';
const CLIENT_KEY = 'gaClientId';
const SESSION_KEY = 'gaSession';

/** GA's web tag ends a session after 30 idle minutes; the panel follows suit so its
 *  sessions read the same way as the site's. */
export const SESSION_TIMEOUT_MS = 30 * 60 * 1000;

/** GA4 counts an event toward users and sessions only when it carries an engagement
 *  time. The panel has no page-visibility timer to measure one, so every event claims a
 *  nominal slice — enough to be counted, too small to inflate engagement figures. */
const ENGAGEMENT_MSEC = 100;

export interface Session {
  id: string;
  lastAt: number;
}

type Params = Record<string, string | number | boolean>;

/** The session an event at `now` belongs to: the previous one while it is still warm,
 *  otherwise a fresh one keyed by its start time in seconds, as gtag.js keys its own. */
export function nextSession(prev: Session | undefined, now: number): Session {
  if (prev && now - prev.lastAt < SESSION_TIMEOUT_MS) {
    return { id: prev.id, lastAt: now };
  }
  return { id: String(Math.floor(now / 1000)), lastAt: now };
}

/** The Measurement Protocol body for one event. */
export function buildPayload(client: string, sessionId: string, name: string, params: Params = {}) {
  return {
    client_id: client,
    events: [
      {
        name,
        params: { ...params, surface: 'extension', session_id: sessionId, engagement_time_msec: ENGAGEMENT_MSEC },
      },
    ],
  };
}

/** An anonymous id for this browser profile, minted once. Not the freehire account id:
 *  GA has no business knowing who the user is, only that it is the same install. */
async function clientId(): Promise<string> {
  const stored = await browser.storage.local.get(CLIENT_KEY);
  const existing = stored[CLIENT_KEY];
  if (typeof existing === 'string') return existing;
  const minted = crypto.randomUUID();
  await browser.storage.local.set({ [CLIENT_KEY]: minted });
  return minted;
}

/** Session state lives in `storage.session`, which Chrome clears with the browser — the
 *  same lifetime a session cookie would have had on the web. */
async function session(): Promise<Session> {
  const stored = await browser.storage.session.get(SESSION_KEY);
  const next = nextSession(stored[SESSION_KEY] as Session | undefined, Date.now());
  await browser.storage.session.set({ [SESSION_KEY]: next });
  return next;
}

/**
 * Record one event. Fire-and-forget: analytics must never fail, slow or block what
 * the user did, so every error — storage, network, a blocked request — is swallowed.
 */
export function track(name: string, params?: Params): void {
  if (!MEASUREMENT_ID || !API_SECRET) return;
  void (async () => {
    try {
      const [cid, sess] = await Promise.all([clientId(), session()]);
      const url = `${ENDPOINT}?measurement_id=${encodeURIComponent(MEASUREMENT_ID)}&api_secret=${encodeURIComponent(API_SECRET)}`;
      await fetch(url, {
        method: 'POST',
        credentials: 'omit',
        keepalive: true,
        body: JSON.stringify(buildPayload(cid, sess.id, name, params)),
      });
    } catch {
      /* best-effort — see above */
    }
  })();
}
