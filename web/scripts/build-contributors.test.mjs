import { describe, expect, it } from 'vitest';
import {
  RECENT_PULL_REQUEST_LIMIT,
  assembleEntries,
  assertUsable,
  serializeSnapshot,
} from './build-contributors.mjs';

const author = (login, over = {}) => ({
  login,
  id: 7,
  avatarUrl: `https://avatars.githubusercontent.com/u/7`,
  accountType: 'User',
  ...over,
});

const pr = (login, number, mergedAt, over = {}) => ({
  author: author(login, over.author),
  number,
  title: `pull request ${number}`,
  mergedAt,
  url: `https://github.com/strelov1/freehire/pull/${number}`,
});

const issue = (login, createdAt, over = {}) => ({
  author: author(login, over.author),
  createdAt,
});

const find = (entries, login) => entries.find((e) => e.login === login);

describe('assembleEntries', () => {
  it('counts a contributor’s merged pull requests and opened issues', () => {
    const entries = assembleEntries({
      pullRequests: [pr('aleganza', 1, '2026-01-01T00:00:00Z'), pr('aleganza', 2, '2026-02-01T00:00:00Z')],
      issues: [issue('aleganza', '2026-03-01T00:00:00Z')],
      admins: [],
    });

    expect(find(entries, 'aleganza')).toMatchObject({ mergedPullRequests: 2, openedIssues: 1 });
  });

  // The dates span both kinds of contribution, so an issue filed after someone's last
  // merged pull request still counts as them being here recently — which is what the
  // page's ordering reads.
  it('spans both pull requests and issues when dating a contributor', () => {
    const entries = assembleEntries({
      pullRequests: [pr('aleganza', 1, '2026-02-01T00:00:00Z')],
      issues: [issue('aleganza', '2026-01-01T00:00:00Z'), issue('aleganza', '2026-03-01T00:00:00Z')],
      admins: [],
    });

    expect(find(entries, 'aleganza')).toMatchObject({
      firstContributionAt: '2026-01-01T00:00:00Z',
      lastContributionAt: '2026-03-01T00:00:00Z',
    });
  });

  it('includes someone who has only ever opened an issue', () => {
    const entries = assembleEntries({
      pullRequests: [],
      issues: [issue('reporter', '2026-01-01T00:00:00Z')],
      admins: [],
    });

    expect(find(entries, 'reporter')).toMatchObject({
      mergedPullRequests: 0,
      openedIssues: 1,
      recentPullRequests: [],
    });
  });

  // The repository carries thousands of merged pull requests, nearly all of them one
  // maintainer's. Writing every one of them would put a large file through a daily
  // commit to restate the same history, so the detail is bounded and the totals are not.
  it('keeps only the most recent pull requests, newest first, but counts them all', () => {
    const many = Array.from({ length: RECENT_PULL_REQUEST_LIMIT + 5 }, (_, i) =>
      pr('strelov1', i + 1, `2026-01-${String(i + 1).padStart(2, '0')}T00:00:00Z`),
    );

    const entry = find(assembleEntries({ pullRequests: many, issues: [], admins: [] }), 'strelov1');

    expect(entry.mergedPullRequests).toBe(RECENT_PULL_REQUEST_LIMIT + 5);
    expect(entry.recentPullRequests).toHaveLength(RECENT_PULL_REQUEST_LIMIT);
    expect(entry.recentPullRequests[0].number).toBe(RECENT_PULL_REQUEST_LIMIT + 5);
  });

  it('marks an admin collaborator as a maintainer and everyone else as a contributor', () => {
    const entries = assembleEntries({
      pullRequests: [pr('strelov1', 1, '2026-01-01T00:00:00Z'), pr('aleganza', 2, '2026-01-02T00:00:00Z')],
      issues: [],
      admins: ['strelov1'],
    });

    expect(find(entries, 'strelov1').role).toBe('maintainer');
    expect(find(entries, 'aleganza').role).toBe('contributor');
  });

  // Collaborator logins and author logins come from different endpoints and GitHub
  // logins are case-insensitive, so the two must not be compared literally.
  it('matches an admin collaborator regardless of case', () => {
    const entries = assembleEntries({
      pullRequests: [pr('Strelov1', 1, '2026-01-01T00:00:00Z')],
      issues: [],
      admins: ['strelov1'],
    });

    expect(find(entries, 'Strelov1').role).toBe('maintainer');
  });

  // Someone may not want their name on a marketing page. An exclusion list fails safe:
  // forgetting to add a login shows them, which is the state they were already in.
  it('leaves out a login that asked to be excluded', () => {
    const entries = assembleEntries({
      pullRequests: [pr('shy', 1, '2026-01-01T00:00:00Z'), pr('aleganza', 2, '2026-01-02T00:00:00Z')],
      issues: [],
      admins: [],
      excluded: ['shy'],
    });

    expect(entries.map((e) => e.login)).toEqual(['aleganza']);
  });

  // A pull request or issue whose author GitHub no longer resolves (a deleted account)
  // comes back without one. It belongs to nobody, so it is counted for nobody.
  it('ignores a contribution with no author', () => {
    const orphan = { author: null, number: 1, title: 'orphan', mergedAt: '2026-01-01T00:00:00Z', url: 'x' };

    const entries = assembleEntries({
      pullRequests: [orphan, pr('aleganza', 2, '2026-01-02T00:00:00Z')],
      issues: [],
      admins: [],
    });

    expect(entries.map((e) => e.login)).toEqual(['aleganza']);
    expect(find(entries, 'aleganza').mergedPullRequests).toBe(1);
  });
});

describe('assertUsable', () => {
  // A collection yielding nobody is a failed measurement, not an empty repository —
  // the same rule the suggestion-dictionary rebuild follows before it swaps an index.
  // Every failure this guards against is silent: a renamed GraphQL field, a query that
  // starts returning an empty connection, a token that reads nothing. The script exits
  // non-zero, writes nothing, and the previously committed snapshot keeps serving; the
  // alternative is a green run that empties the page.
  it('refuses a collection that found nobody', () => {
    expect(() => assertUsable([])).toThrow(/no human contributors/);
  });

  it('refuses a collection with no human in it', () => {
    const onlyBots = assembleEntries({
      pullRequests: [pr('dependabot', 1, '2026-01-01T00:00:00Z', { author: { accountType: 'Bot' } })],
      issues: [],
      admins: [],
    });

    expect(() => assertUsable(onlyBots)).toThrow(/no human contributors/);
  });

  it('accepts a collection with at least one human', () => {
    const found = assembleEntries({
      pullRequests: [pr('aleganza', 1, '2026-01-01T00:00:00Z')],
      issues: [],
      admins: [],
    });

    expect(() => assertUsable(found)).not.toThrow();
  });
});

describe('serializeSnapshot', () => {
  // The workflow decides whether to commit by asking git whether the file changed, so
  // an unchanged collection has to produce byte-identical output. Object key order in
  // JSON.stringify follows insertion order, and insertion order here follows whatever
  // order GitHub happened to page the results in.
  it('writes people in a stable order whatever order they were collected in', () => {
    // The pull-request number is derived from the login, not from the position, so the
    // two collections differ only in the order GitHub returned them.
    const number = { ada: 11, bea: 22, cy: 33 };
    const collected = (order) =>
      serializeSnapshot(
        assembleEntries({
          pullRequests: order.map((login) => pr(login, number[login], '2026-01-01T00:00:00Z')),
          issues: [],
          admins: [],
        }),
      );

    const written = collected(['bea', 'ada', 'cy']);

    expect(written).toBe(collected(['cy', 'bea', 'ada']));
    expect(JSON.parse(written).people.map((p) => p.login)).toEqual(['ada', 'bea', 'cy']);
  });

  // THE FILE CARRIES NO TIMESTAMP, AND THAT IS THE POINT. The workflow decides whether
  // to commit by asking git whether this file changed; a collected-at stamp changes on
  // every run, so it would make every run a commit — and, since the host deploys a green
  // main, a daily production deploy of nothing. When the data last changed is already
  // recorded by the commit that changed it, exactly and unforgeably.
  it('carries nothing that changes between runs of identical data', () => {
    const entries = assembleEntries({
      pullRequests: [pr('aleganza', 1, '2026-01-01T00:00:00Z')],
      issues: [],
      admins: [],
    });

    expect(serializeSnapshot(entries)).toBe(serializeSnapshot(entries));
    expect(JSON.parse(serializeSnapshot(entries))).toEqual({
      people: [expect.objectContaining({ login: 'aleganza' })],
    });
  });

  it('ends with a newline so the committed file is a well-formed text file', () => {
    expect(serializeSnapshot([]).endsWith('\n')).toBe(true);
  });
});
