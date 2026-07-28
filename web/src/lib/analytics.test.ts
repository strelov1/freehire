import { describe, expect, it } from 'vitest';
import { initTrackers, isPrivateRoute, track, isFeatureEnabled } from './analytics';

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
