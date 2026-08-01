import { describe, expect, it } from 'vitest';
import {
  claimSignupOnce,
  cvUploadReason,
  initTrackers,
  isFreshAccount,
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
