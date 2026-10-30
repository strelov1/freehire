import { describe, it, expect } from 'vitest';
import {
  accountNav,
  activeSectionHref,
  isSectionActive,
  visibleAccountNav,
  visibleWorkspaces,
  workspaceFor,
  workspaceNav,
} from './accountNav';

describe('accountNav config', () => {
  it('lists the twenty-three account sections', () => {
    expect(accountNav).toHaveLength(23);
  });

  // Submissions and the invite link are tabs of Contributions and Plan now; their old
  // sections redirect.
  it('reaches submissions and the invite link through their parent sections', () => {
    const hrefs: string[] = accountNav.map((i) => i.href);
    expect(hrefs).toEqual(expect.arrayContaining(['/my/contributions', '/my/plan']));
    expect(hrefs).not.toContain('/my/submissions');
    expect(hrefs).not.toContain('/my/invite');
  });

  // API keys and the webhook are tabs of Integrations now; their old sections redirect.
  it('reaches API keys and the webhook through Integrations, not sections of their own', () => {
    const hrefs: string[] = accountNav.map((i) => i.href);
    expect(hrefs).toContain('/my/integrations');
    expect(hrefs).not.toContain('/my/api-keys');
    expect(hrefs).not.toContain('/my/webhook');
  });

  // A section whose page exists and whose nav entry does not is reachable by URL and by
  // nothing else — the shape of bug where "the feature isn't there" and "I can't get to
  // it with the mouse" look identical from the outside.
  it('offers a section for mentorship sessions', () => {
    expect(accountNav.map((i) => i.href)).toContain('/my/mentorship');
  });

  // Beta-gated while the marketplace has no supply. Asserted from BOTH sides: an entry
  // that is hidden from everybody is the same bug as one shown to everybody, and only the
  // pair of assertions tells them apart.
  it('shows mentorship to a beta tester and to nobody else', () => {
    const hrefs = (mod: boolean, beta: boolean) => visibleAccountNav(mod, beta).map((i) => i.href);
    expect(hrefs(false, true)).toContain('/my/mentorship');
    expect(hrefs(false, false)).not.toContain('/my/mentorship');
    expect(hrefs(true, false)).not.toContain('/my/mentorship');
  });

  it('offers a security section for email, password and session management', () => {
    expect(accountNav.map((i) => i.href)).toContain('/my/account');
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
  // The Talent Network was reachable only from the profile layout's invitation card, and
  // that card is dismissible now — so without a section of its own, closing a banner and
  // losing the feature would be the same gesture. Open to everyone, beta or not.
  it('offers the Talent Network to everyone, gate-free', () => {
    for (const [mod, beta] of [
      [false, false],
      [true, false],
      [false, true],
      [true, true],
    ] as const) {
      expect(visibleAccountNav(mod, beta).map((i) => i.href)).toContain('/my/talent-network');
    }
  });

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

// The account area is one account seen through three workspaces. The sidebar shows the
// current one only, so what decides "current" is the whole feature.
describe('workspaceFor', () => {
  it('puts the mentor panes in the Mentor workspace', () => {
    expect(workspaceFor('/my/mentorship/bookings')).toBe('mentor');
    expect(workspaceFor('/my/mentorship/schedule')).toBe('mentor');
    expect(workspaceFor('/my/mentorship/profile')).toBe('mentor');
  });

  // The bare section is the member's OWN booked sessions — they are a mentee there, so
  // the most specific match must win over the shared /my/mentorship prefix.
  it("keeps the member's own sessions in Job search", () => {
    expect(workspaceFor('/my/mentorship')).toBe('seeker');
  });

  // A session is read by both parties, so it has an address per role: one shared address
  // dropped a mentor into Job search the moment they opened a booking.
  it("keeps a session in the workspace of the side that opened it", () => {
    expect(workspaceFor('/my/mentorship/bookings/abc123')).toBe('mentor');
    expect(workspaceFor('/my/mentorship/sessions/abc123')).toBe('seeker');
  });

  it('puts the company pages in the Employer workspace', () => {
    expect(workspaceFor('/my/employer')).toBe('employer');
    expect(workspaceFor('/my/employer/vacancies')).toBe('employer');
  });

  // Referrals was one page for two roles: the member asking for a referral, and the
  // insider offering them. Asking stays in Job search; offering is its own workspace.
  it('splits referrals between asking (Job search) and offering (Referrer)', () => {
    expect(workspaceFor('/my/referrals')).toBe('seeker');
    expect(workspaceFor('/my/referrals/offers')).toBe('referrer');
    expect(workspaceFor('/my/referrals/incoming')).toBe('referrer');
  });

  it('defaults to Job search for its own sections, shared settings and unknown paths', () => {
    expect(workspaceFor('/my/tracking/pipeline')).toBe('seeker');
    expect(workspaceFor('/my/account')).toBe('seeker');
    expect(workspaceFor('/my/searches')).toBe('seeker');
  });
});

const SETTINGS = ['/my/notifications', '/my/integrations', '/my/contributions', '/my/plan', '/my/account'];

describe('workspaceNav', () => {
  const groups = (ws: Parameters<typeof workspaceNav>[0], beta = true) =>
    workspaceNav(ws, false, beta).map((g) => [g.group, g.items.map((i) => i.href)]);

  it('groups Job search: everyday, anonymous faces, help, settings', () => {
    expect(groups('seeker')).toEqual([
      [
        'main',
        [
          '/my/profile',
          '/my/activity',
          '/my/tracking',
          '/my/inbox',
          '/my/market-pulse',
          '/my/assistant',
          '/my/cvs',
        ],
      ],
      ['found', ['/my/talent-network', '/my/buddies']],
      ['help', ['/my/referrals', '/my/mentorship']],
      ['settings', SETTINGS],
    ]);
  });

  it('shows the Mentor panes and the shared settings, nothing of Job search', () => {
    expect(groups('mentor')).toEqual([
      ['main', ['/my/mentorship/bookings', '/my/mentorship/schedule', '/my/mentorship/profile']],
      ['settings', SETTINGS],
    ]);
  });

  it('shows the referrer panes and the shared settings', () => {
    expect(groups('referrer')).toEqual([
      ['main', ['/my/referrals/offers', '/my/referrals/incoming']],
      ['settings', SETTINGS],
    ]);
  });

  it('shows the Employer company pages and the shared settings', () => {
    expect(groups('employer')).toEqual([
      ['main', ['/my/employer', '/my/employer/vacancies']],
      ['settings', SETTINGS],
    ]);
  });

  // A group the gate empties must disappear rather than render as a lone heading.
  it('drops the beta-only mentorship entry, and never leaves an empty group', () => {
    const help = workspaceNav('seeker', false, false).find((g) => g.group === 'help');
    expect(help?.items.map((i) => i.href)).toEqual(['/my/referrals']);
    expect(workspaceNav('mentor', false, false).map((g) => g.group)).toEqual(['settings']);
  });
});

describe('visibleWorkspaces', () => {
  it('offers Mentor to a beta tester only, the rest to everyone', () => {
    expect(visibleWorkspaces(false).map((w) => w.id)).toEqual(['seeker', 'referrer', 'employer']);
    expect(visibleWorkspaces(true).map((w) => w.id)).toEqual(['seeker', 'mentor', 'referrer', 'employer']);
  });

  it("opens each workspace on one of its own sections", () => {
    for (const w of visibleWorkspaces(true)) expect(workspaceFor(w.home)).toBe(w.id);
  });
});

describe('activeSectionHref', () => {
  const all = accountNav;

  it('marks the most specific section, not every ancestor', () => {
    expect(activeSectionHref('/my/mentorship/schedule', all)).toBe('/my/mentorship/schedule');
    expect(activeSectionHref('/my/mentorship', all)).toBe('/my/mentorship');
  });

  it('covers descendants and misses unknown paths', () => {
    expect(activeSectionHref('/my/notifications/searches', all)).toBe('/my/notifications');
    expect(activeSectionHref('/my/searches', all)).toBeUndefined();
  });
});
