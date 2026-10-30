import { describe, expect, it, vi } from 'vitest';
import {
  claimSignupOnce,
  cvUploadReason,
  initTrackers,
  isFreshAccount,
  isHandoffReferrer,
  isPrivateRoute,
  track,
  isFeatureEnabled,
} from './analytics';

// These tests exercise the pure/guarded surface — no PostHog init happens (the
// config below carries no key), so the module is in its uninitialized state
// throughout — plus the shape of what the GA bootstrap queues, which is the one
// SDK side effect worth asserting: getting it wrong fails silently (see below).

describe('isPrivateRoute', () => {
  it('treats /my and its subtree as private', () => {
    expect(isPrivateRoute('/my')).toBe(true);
    expect(isPrivateRoute('/my/inbox')).toBe(true);
    expect(isPrivateRoute('/my/tracking')).toBe(true);
  });

  it('treats the buddies space as private: it holds private conversations', () => {
    expect(isPrivateRoute('/buddies')).toBe(true);
    expect(isPrivateRoute('/buddies/chats')).toBe(true);
    expect(isPrivateRoute('/buddies/amber-heron-42')).toBe(true);
  });

  it('treats public routes as not private', () => {
    expect(isPrivateRoute('/')).toBe(false);
    expect(isPrivateRoute('/jobs')).toBe(false);
    expect(isPrivateRoute('/companies/acme')).toBe(false);
    // A route that merely starts with the letters "my" is not under /my.
    expect(isPrivateRoute('/mystery')).toBe(false);
  });
});

describe('track (uninitialized)', () => {
  it('is a safe no-op when PostHog is not initialized', () => {
    expect(() => track('job_apply', { slug: 'x', source: 'greenhouse' })).not.toThrow();
  });
});

describe('isFeatureEnabled (uninitialized)', () => {
  it('returns the caller-supplied fallback when PostHog is inert', () => {
    expect(isFeatureEnabled('some-flag', true)).toBe(true);
    expect(isFeatureEnabled('other-flag', false)).toBe(false);
  });
});

describe('cvUploadReason', () => {
  // The server's rejection is prose shown to the user, so the mapper exists to keep
  // the metric stable when that copy is reworded — the codes are what PostHog sees.
  it('recognizes the scan-PDF rejection', () => {
    expect(
      cvUploadReason(
        "couldn't read any text from this PDF — it looks like a scan or image. Upload a text-based PDF, or paste your résumé text instead.",
      ),
    ).toBe('no_text');
  });

  it('recognizes the other upload rejections', () => {
    expect(cvUploadReason('missing resume file')).toBe('missing_file');
    expect(cvUploadReason('cannot read resume file')).toBe('unreadable');
    expect(cvUploadReason('invalid request body')).toBe('bad_request');
  });

  it('matches regardless of case and surrounding whitespace', () => {
    expect(cvUploadReason('  Missing Resume File  ')).toBe('missing_file');
  });

  it('falls back to a catch-all so a failure is never dropped from the count', () => {
    expect(cvUploadReason('request entity too large')).toBe('other');
    expect(cvUploadReason('')).toBe('other');
  });
});

describe('isFreshAccount', () => {
  // OAuth sign-up is a full-page redirect, so the browser cannot tell a first-ever
  // sign-in from the hundredth; a just-created account is the only signal it has.
  const now = Date.parse('2026-08-01T12:00:00Z');
  const windowMs = 2 * 60 * 1000;

  it('treats an account created inside the window as fresh', () => {
    expect(isFreshAccount('2026-08-01T11:59:00Z', now, windowMs)).toBe(true);
  });

  it('treats an older account as not fresh', () => {
    expect(isFreshAccount('2026-08-01T11:50:00Z', now, windowMs)).toBe(false);
  });

  it('is exclusive at the window edge', () => {
    expect(isFreshAccount('2026-08-01T11:58:00Z', now, windowMs)).toBe(false);
  });

  it('tolerates a clock skew that puts creation slightly in the future', () => {
    expect(isFreshAccount('2026-08-01T12:00:30Z', now, windowMs)).toBe(true);
  });

  it('treats a missing or unparseable timestamp as not fresh', () => {
    expect(isFreshAccount(null, now, windowMs)).toBe(false);
    expect(isFreshAccount('not a date', now, windowMs)).toBe(false);
  });
});

describe('claimSignupOnce', () => {
  // A reload inside the freshness window would re-fire `signup` for the same
  // account, so the claim has to be idempotent per user.
  function memoryStorage() {
    const data = new Map<string, string>();
    return {
      getItem: (k: string) => data.get(k) ?? null,
      setItem: (k: string, v: string) => void data.set(k, v),
    };
  }

  it('claims once and refuses every repeat for the same account', () => {
    const store = memoryStorage();
    expect(claimSignupOnce(42, store)).toBe(true);
    expect(claimSignupOnce(42, store)).toBe(false);
    expect(claimSignupOnce(42, store)).toBe(false);
  });

  it('claims separately per account', () => {
    const store = memoryStorage();
    expect(claimSignupOnce(1, store)).toBe(true);
    expect(claimSignupOnce(2, store)).toBe(true);
  });

  it('refuses when storage is unavailable, rather than risking a double count', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
      setItem: () => {
        throw new Error('denied');
      },
    };
    expect(claimSignupOnce(7, broken)).toBe(false);
  });
});

describe('Google Analytics bootstrap', () => {
  it('queues gtag commands as Arguments, not as a plain Array', () => {
    // The bootstrap only needs a non-localhost hostname, a <script> object to
    // fill in, and a head to append it to — stubbed here so it can run in the
    // plain-node test environment.
    Object.assign(globalThis, {
      window: globalThis,
      location: { hostname: 'freehire.me' },
      document: { createElement: () => ({}), head: { appendChild: () => {} } },
    });

    initTrackers({ key: '', apiHost: '/ingest' });

    const queued = (globalThis as { dataLayer?: unknown[] }).dataLayer ?? [];
    expect(queued).toHaveLength(2); // gtag('js', …) and gtag('config', …)
    // gtag.js recognizes a dataLayer entry as a command ONLY when it is an
    // Arguments object; a plain Array is silently treated as a GTM-style push
    // and ignored, so the tag loads and registers but never sends a hit.
    for (const entry of queued) {
      expect(Object.prototype.toString.call(entry)).toBe('[object Arguments]');
    }
  });
});

describe('track → Google Analytics', () => {
  // A fresh module per test: whether GA has been started is module state, and
  // the bootstrap test above has already started it in the shared copy.
  async function freshAnalytics() {
    vi.resetModules();
    Object.assign(globalThis, {
      window: globalThis,
      location: { hostname: 'freehire.me' },
      document: { createElement: () => ({}), head: { appendChild: () => {} } },
      dataLayer: [],
    });
    return import('./analytics');
  }
  const queued = () => (globalThis as { dataLayer?: unknown[] }).dataLayer ?? [];
  const lastCommand = () => Array.from(queued().at(-1) as ArrayLike<unknown>);

  it('sends nothing to GA before the trackers are started', async () => {
    const analytics = await freshAnalytics();
    analytics.track('job_apply', { slug: 'x' });
    expect(queued()).toHaveLength(0);
  });

  it('forwards an event under its own name once GA is running', async () => {
    const analytics = await freshAnalytics();
    analytics.initTrackers({ key: '', apiHost: '/ingest' });
    analytics.track('job_apply', { slug: 'x', source: 'greenhouse' });
    expect(lastCommand()).toEqual(['event', 'job_apply', { slug: 'x', source: 'greenhouse' }]);
    expect(Object.prototype.toString.call(queued().at(-1))).toBe('[object Arguments]');
  });

  it('renames the funnel steps to the events GA4 reports on', async () => {
    const analytics = await freshAnalytics();
    analytics.initTrackers({ key: '', apiHost: '/ingest' });
    analytics.track('signup', { method: 'oauth' });
    expect(lastCommand()).toEqual(['event', 'sign_up', { method: 'oauth' }]);
    analytics.track('checkout_start', { price_id: 'default' });
    expect(lastCommand()).toEqual(['event', 'begin_checkout', { price_id: 'default' }]);
    analytics.track('subscribe', { plan: 'pro' });
    expect(lastCommand()).toEqual(['event', 'purchase', { plan: 'pro' }]);
  });

  it('keeps the typed search text out of GA', async () => {
    const analytics = await freshAnalytics();
    analytics.initTrackers({ key: '', apiHost: '/ingest' });
    analytics.track('search', { q: 'jane.doe@example.com', facets: 2 });
    expect(lastCommand()).toEqual(['event', 'search', { facets: 2 }]);
  });
});

describe('isHandoffReferrer', () => {
  it('recognizes the OAuth and checkout hosts that hand a visitor back', () => {
    expect(isHandoffReferrer('https://accounts.google.com/')).toBe(true);
    expect(isHandoffReferrer('https://appleid.apple.com/auth/authorize')).toBe(true);
    expect(isHandoffReferrer('https://checkout.stripe.com/c/pay/cs_live_x')).toBe(true);
  });

  it('leaves real traffic sources alone', () => {
    expect(isHandoffReferrer('https://chatgpt.com/')).toBe(false);
    expect(isHandoffReferrer('https://www.google.com/')).toBe(false);
    // Only an exact host matches, never a look-alike suffix.
    expect(isHandoffReferrer('https://accounts.google.com.evil.example/')).toBe(false);
  });

  it('treats an empty or unparseable referrer as not a handoff', () => {
    expect(isHandoffReferrer('')).toBe(false);
    expect(isHandoffReferrer(undefined)).toBe(false);
    expect(isHandoffReferrer('not a url')).toBe(false);
  });
});
