import { describe, expect, it } from 'vitest';
import { contributorGroups, findContributor } from './contributors';
import type { ContributorEntry, ContributorsSnapshot } from './contributors';
import committed from './data/contributors.json';

function person(over: Partial<ContributorEntry> = {}): ContributorEntry {
  return {
    login: 'someone',
    id: 1,
    avatarUrl: 'https://avatars.githubusercontent.com/u/1',
    accountType: 'User',
    role: 'contributor',
    firstContributionAt: '2026-01-01T00:00:00Z',
    lastContributionAt: '2026-01-01T00:00:00Z',
    mergedPullRequests: 1,
    openedIssues: 0,
    recentPullRequests: [],
    ...over,
  };
}

function snapshot(people: ContributorEntry[]): ContributorsSnapshot {
  return { generatedAt: '2026-09-07T00:00:00Z', people };
}

const logins = (entries: ContributorEntry[]) => entries.map((e) => e.login);

describe('contributorGroups', () => {
  it('shows a human contributor', () => {
    const groups = contributorGroups(snapshot([person({ login: 'aleganza' })]));

    expect(logins(groups.contributors)).toEqual(['aleganza']);
  });

  // Left in, dependabot is the second-largest contributor to this repository and would
  // sit near the top of a page about people. The account type is what GitHub itself
  // reports, and the login suffix catches an account the collector saw without one.
  it('drops an account GitHub reports as a bot', () => {
    const groups = contributorGroups(
      snapshot([person({ login: 'aleganza' }), person({ login: 'renovate', accountType: 'Bot' })]),
    );

    expect(logins(groups.contributors)).toEqual(['aleganza']);
  });

  it('drops an account whose login is marked as a bot', () => {
    const groups = contributorGroups(
      snapshot([person({ login: 'aleganza' }), person({ login: 'dependabot[bot]' })]),
    );

    expect(logins(groups.contributors)).toEqual(['aleganza']);
  });

  // One account holds roughly three thousand commits here and every other holds fewer
  // than ten. Rendered in one grid, the eight smaller histories read as noise beside
  // the one large one, so the maintainer is shown apart rather than at the top.
  it('puts a maintainer in their own group, never among the contributors', () => {
    const groups = contributorGroups(
      snapshot([person({ login: 'strelov1', role: 'maintainer' }), person({ login: 'aleganza' })]),
    );

    expect(logins(groups.maintainers)).toEqual(['strelov1']);
    expect(logins(groups.contributors)).toEqual(['aleganza']);
  });

  it('drops a bot that carries maintainer permissions', () => {
    const groups = contributorGroups(
      snapshot([
        person({ login: 'strelov1', role: 'maintainer' }),
        person({ login: 'github-actions[bot]', role: 'maintainer' }),
      ]),
    );

    expect(logins(groups.maintainers)).toEqual(['strelov1']);
  });

  // The page exists to attract the next contributor, so the newest one must be the one
  // it shows first. Ordered by volume, a newcomer is buried the moment they arrive
  // behind everyone who has been here longer — the opposite of the page's purpose.
  it('orders contributors by their most recent contribution, newest first', () => {
    const groups = contributorGroups(
      snapshot([
        person({
          login: 'long-timer',
          mergedPullRequests: 8,
          lastContributionAt: '2025-09-01T00:00:00Z',
        }),
        person({
          login: 'newcomer',
          mergedPullRequests: 1,
          lastContributionAt: '2026-09-01T00:00:00Z',
        }),
      ]),
    );

    expect(logins(groups.contributors)).toEqual(['newcomer', 'long-timer']);
  });

  it('orders an issue reporter with no merged code ahead of an older code contributor', () => {
    const groups = contributorGroups(
      snapshot([
        person({
          login: 'coder',
          mergedPullRequests: 8,
          lastContributionAt: '2025-09-01T00:00:00Z',
        }),
        person({
          login: 'reporter',
          mergedPullRequests: 0,
          openedIssues: 1,
          lastContributionAt: '2026-09-01T00:00:00Z',
        }),
      ]),
    );

    expect(logins(groups.contributors)).toEqual(['reporter', 'coder']);
  });

  // A merged pull request is not the only way to have helped. Someone who has only ever
  // filed issues is shown, and their profile simply has nothing in its pull-request list
  // rather than being absent from the site.
  it('keeps a contributor whose only contribution is an issue', () => {
    const groups = contributorGroups(
      snapshot([
        person({ login: 'reporter', mergedPullRequests: 0, openedIssues: 3, recentPullRequests: [] }),
      ]),
    );

    expect(logins(groups.contributors)).toEqual(['reporter']);
    expect(groups.contributors[0].recentPullRequests).toEqual([]);
  });

  // The collector reaches accounts through several queries, so an entry can survive with
  // nothing attached to it — a review comment author, a closed pull request that never
  // merged. Nothing is a contribution the page can name, so the entry is not shown.
  it('drops an entry with no merged pull request and no opened issue', () => {
    const groups = contributorGroups(
      snapshot([
        person({ login: 'passer-by', mergedPullRequests: 0, openedIssues: 0 }),
        person({ login: 'aleganza' }),
      ]),
    );

    expect(logins(groups.contributors)).toEqual(['aleganza']);
  });
});

// The collector and these rules are two halves of one feature that never run together
// anywhere else — the collector runs in a scheduled job, the rules run in a page. This
// is where they are checked against each other, on the file that actually ships.
//
// It earns its place: the GraphQL API spells dependabot's login `dependabot`, with no
// `[bot]` suffix, while the REST endpoint spells the same account `dependabot[bot]`. A
// bot filter written against the obvious signal alone would put GitHub's second-largest
// contributor to this repository on a page about people, and every fixture in this file
// would still be green.
describe('the committed snapshot', () => {
  const snapshotOnDisk = committed as ContributorsSnapshot;

  it('carries at least one bot for the rules to have something to exclude', () => {
    const bots = snapshotOnDisk.people.filter((p) => p.accountType !== 'User');

    expect(bots.length).toBeGreaterThan(0);
  });

  it('shows no bot on either group', () => {
    const groups = contributorGroups(snapshotOnDisk);
    const shown = [...groups.maintainers, ...groups.contributors];

    expect(shown.filter((p) => p.accountType !== 'User')).toEqual([]);
    expect(shown.filter((p) => p.login.toLowerCase().includes('bot'))).toEqual([]);
  });

  it('has both a maintainer and contributors to render', () => {
    const groups = contributorGroups(snapshotOnDisk);

    expect(groups.maintainers.length).toBeGreaterThan(0);
    expect(groups.contributors.length).toBeGreaterThan(0);
  });
});

describe('findContributor', () => {
  it('finds a shown contributor by their login', () => {
    const found = findContributor(snapshot([person({ login: 'aleganza' })]), 'aleganza');

    expect(found?.login).toBe('aleganza');
  });

  it('does not find a login absent from the snapshot', () => {
    expect(findContributor(snapshot([person({ login: 'aleganza' })]), 'nobody')).toBeNull();
  });

  // The lookup must apply the same rules the showcase does. Reading the raw snapshot
  // instead would give a bot — or an entry with nothing to show — a profile page and a
  // social card that the showcase itself never links to.
  it('does not find a bot, even though the snapshot carries it', () => {
    const people = snapshot([person({ login: 'dependabot[bot]' })]);

    expect(findContributor(people, 'dependabot[bot]')).toBeNull();
  });

  it('does not find an entry with nothing to show', () => {
    const people = snapshot([
      person({ login: 'passer-by', mergedPullRequests: 0, openedIssues: 0 }),
    ]);

    expect(findContributor(people, 'passer-by')).toBeNull();
  });

  // GitHub logins are case-insensitive, so a link typed or shared with different casing
  // must reach the same page rather than a 404.
  it('matches a login regardless of case', () => {
    const found = findContributor(snapshot([person({ login: 'aleganza' })]), 'AlEgAnZa');

    expect(found?.login).toBe('aleganza');
  });
});
