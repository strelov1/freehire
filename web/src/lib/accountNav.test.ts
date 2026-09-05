import { describe, it, expect } from 'vitest';
import { accountNav, isSectionActive, visibleAccountNav } from './accountNav';

describe('accountNav config', () => {
  it('lists the sixteen account sections', () => {
    expect(accountNav).toHaveLength(16);
  });

  it('offers a security section for password and session management', () => {
    expect(accountNav.map((i) => i.href)).toContain('/my/security');
  });

  it('leads with the four everyday sections in use order', () => {
    const hrefs = accountNav.map((i) => i.href);
    expect(hrefs.slice(0, 4)).toEqual(['/my/profile', '/my/activity', '/my/tracking', '/my/inbox']);
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

describe('visibleAccountNav', () => {
  it('shows the Agent, Inbox and Tailor sections to a plain user', () => {
    const hrefs = visibleAccountNav(false, false).map((i) => i.href);
    // The Assistant left its beta rollout: it runs on the user's own machine
    // with their own Claude, so there is nothing left to ration.
    expect(hrefs).toContain('/my/assistant');
    expect(hrefs).toContain('/my/inbox');
    expect(hrefs).toContain('/my/cvs'); // the tailoring list is public now (the plan meters the AI spend)
  });

  it('shows the tailoring section to every signed-in user, gate-free', () => {
    for (const [mod, beta] of [
      [false, false],
      [true, false],
      [false, true],
    ] as const) {
      expect(visibleAccountNav(mod, beta).map((i) => i.href)).toContain('/my/cvs');
    }
  });

  it('shows the Assistant regardless of role or beta membership', () => {
    for (const [mod, beta] of [
      [false, false],
      [true, false],
      [false, true],
    ] as const) {
      const hrefs = visibleAccountNav(mod, beta).map((i) => i.href);
      expect(hrefs).toContain('/my/assistant');
      expect(hrefs).toContain('/my/inbox');
    }
  });

  it('shows every section to a moderator who is also a beta tester', () => {
    const hrefs = visibleAccountNav(true, true).map((i) => i.href);
    expect(hrefs).toContain('/my/inbox');
    expect(hrefs).toContain('/my/assistant');
    expect(hrefs).toHaveLength(accountNav.length);
  });

  it('shows Market Pulse to everyone, gate-free', () => {
    for (const [mod, beta] of [
      [false, false],
      [true, false],
      [false, true],
      [true, true],
    ] as const) {
      expect(visibleAccountNav(mod, beta).map((i) => i.href)).toContain('/my/market-pulse');
    }
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

describe('the burger menu duplicates what is opened from anywhere', () => {
  it('names the tailoring section after what it is for', () => {
    // "CV builder" described the tool this grew out of; every CV in here is aimed at one
    // vacancy, which is what the section is actually for.
    const tailor = accountNav.find((i) => i.href === '/my/cvs');
    expect(tailor?.label).toBe('Tailor');
  });
});
