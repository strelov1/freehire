import { describe, expect, it } from 'vitest';
import { isPrivateRoute, track, isFeatureEnabled } from './analytics';

// These tests exercise only the pure/guarded surface — no PostHog init happens,
// so the module is in its uninitialized state throughout.

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
