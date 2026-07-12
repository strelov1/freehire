import { describe, it, expect } from 'vitest';
import { accountNav, isSectionActive } from './accountNav';

describe('accountNav config', () => {
  it('lists the seven account sections', () => {
    expect(accountNav).toHaveLength(7);
  });

  it('places Activity directly after Tracking', () => {
    const hrefs = accountNav.map((i) => i.href);
    expect(hrefs.indexOf('/my/activity')).toBe(hrefs.indexOf('/my/tracking') + 1);
    expect(accountNav.find((i) => i.href === '/my/activity')?.label).toBe('Activity');
  });

  it('every item links under /my/ and has a label', () => {
    for (const item of accountNav) {
      expect(item.href.startsWith('/my/')).toBe(true);
      expect(item.label.length).toBeGreaterThan(0);
    }
  });

  it('has unique hrefs', () => {
    const hrefs = accountNav.map((i) => i.href);
    expect(new Set(hrefs).size).toBe(hrefs.length);
  });
});

describe('isSectionActive', () => {
  it('is active on an exact path match', () => {
    expect(isSectionActive('/my/profile', '/my/profile')).toBe(true);
  });

  it('is active on a descendant path', () => {
    expect(isSectionActive('/my/tracking/pipeline', '/my/tracking')).toBe(true);
  });

  it('is not active on a different section', () => {
    expect(isSectionActive('/my/profile', '/my/tracking')).toBe(false);
  });

  it('does not match on a shared prefix that is not a path segment', () => {
    // '/my/api-keys-extra' shares the '/my/api-keys' prefix but is a different
    // route — a segment boundary ('/') is required, not a bare string prefix.
    expect(isSectionActive('/my/api-keys-extra', '/my/api-keys')).toBe(false);
  });
});
