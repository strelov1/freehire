// Collects the repository's contributors from the GitHub API and writes the snapshot
// the /contributors pages read.
//
// Plain Node on purpose: the scheduled workflow runs `node` against this file with no
// install and no build step, which is what keeps a daily job from depending on the
// whole web toolchain being restored first.
//
// This file does I/O and assembly, and nothing else. Every rule about who is SHOWN and
// in what order lives in web/src/lib/contributors.ts, tested there — see its header for
// why. What is tested here is the assembly around the fetching: the totals, the bounded
// detail, and the stable key order the workflow's commit decision depends on.

import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const REPO = { owner: 'strelov1', name: 'freehire' };

/** Logins to leave out of the snapshot entirely.
 *
 *  Someone may not want their name on a page the site markets itself with, and this is
 *  how that is honoured. It is a hand-written list, which this repository generally
 *  distrusts — but the hazard of a hand-written list is that it hides people who should
 *  be there, and this one fails the other way: forgetting to add a login shows them,
 *  which is the state they were already in. */
const EXCLUDED_LOGINS = [];

const SNAPSHOT_PATH = fileURLToPath(new URL('../src/lib/data/contributors.json', import.meta.url));

/** How many of a person's merged pull requests the snapshot carries in full.
 *
 *  The repository holds thousands of them, nearly all one maintainer's. Writing every
 *  one would put a large file through a daily commit to restate the same history. The
 *  totals stay exact; only the detail is bounded, and the bound is uniform so nothing
 *  about the file changes shape when a second maintainer appears. */
export const RECENT_PULL_REQUEST_LIMIT = 20;

/** Widens a person's contribution span to include this moment.
 *
 *  The span covers both kinds of contribution. An issue filed after someone's last
 *  merged pull request is still them being here recently, and recency is what the page's
 *  ordering reads. */
function dateSeen(entry, at) {
  if (!entry.firstContributionAt || at < entry.firstContributionAt) entry.firstContributionAt = at;
  if (!entry.lastContributionAt || at > entry.lastContributionAt) entry.lastContributionAt = at;
}

/** Turns collected pull requests and issues into one entry per person.
 *
 *  Keyed by the lowercased login, because the same person arrives from three endpoints
 *  (pull requests, issues, collaborators) and GitHub logins are case-insensitive — keyed
 *  literally, one contributor would become two entries and neither would be right. The
 *  login the entry carries is the one GitHub spells it with. */
export function assembleEntries({ pullRequests, issues, admins, excluded = [] }) {
  const isAdmin = new Set(admins.map((login) => login.toLowerCase()));
  const isExcluded = new Set(excluded.map((login) => login.toLowerCase()));
  const byLogin = new Map();

  /** The entry for this author, created on first sight. Returns null for a contribution
   *  that belongs to nobody: a deleted account resolves to no author, and a
   *  contribution with no author is counted for no one. */
  const entryFor = (author) => {
    if (!author) return null;
    const key = author.login.toLowerCase();
    if (isExcluded.has(key)) return null;

    let entry = byLogin.get(key);
    if (!entry) {
      entry = {
        login: author.login,
        id: author.id,
        avatarUrl: author.avatarUrl,
        accountType: author.accountType,
        role: isAdmin.has(key) ? 'maintainer' : 'contributor',
        firstContributionAt: null,
        lastContributionAt: null,
        mergedPullRequests: 0,
        openedIssues: 0,
        recentPullRequests: [],
      };
      byLogin.set(key, entry);
    }
    return entry;
  };

  for (const request of pullRequests) {
    const entry = entryFor(request.author);
    if (!entry) continue;

    entry.mergedPullRequests += 1;
    entry.recentPullRequests.push({
      number: request.number,
      title: request.title,
      mergedAt: request.mergedAt,
      url: request.url,
    });
    dateSeen(entry, request.mergedAt);
  }

  for (const opened of issues) {
    const entry = entryFor(opened.author);
    if (!entry) continue;

    entry.openedIssues += 1;
    dateSeen(entry, opened.createdAt);
  }

  for (const entry of byLogin.values()) {
    entry.recentPullRequests.sort((a, b) => b.mergedAt.localeCompare(a.mergedAt));
    entry.recentPullRequests = entry.recentPullRequests.slice(0, RECENT_PULL_REQUEST_LIMIT);
  }

  return [...byLogin.values()];
}

/** Throws unless the collection found at least one human.
 *
 *  A collection yielding nobody is a failed measurement, not an empty repository — the
 *  same rule the suggestion-dictionary rebuild follows before it swaps an index. Every
 *  failure this guards against is silent: a renamed GraphQL field, a connection that
 *  starts coming back empty, a token that can read nothing. Without it the run writes an
 *  empty file, the workflow commits it, and the page goes blank on a green build.
 *
 *  Humans specifically, because a snapshot of nothing but bots renders as an empty page
 *  once the display rules have had their say. */
export function assertUsable(entries) {
  const humans = entries.filter((e) => e.accountType !== 'Bot' && !e.login.endsWith('[bot]'));
  if (humans.length === 0) {
    throw new Error(
      `collection found no human contributors (${entries.length} entries in total) — refusing to write a snapshot that would empty the page`,
    );
  }
}

/** The snapshot as it is written to disk.
 *
 *  EVERYTHING HERE IS A FUNCTION OF THE DATA AND NOTHING ELSE, because the workflow
 *  decides whether to commit by asking git whether this file changed — so anything that
 *  varies between runs of identical data makes every run a commit, and (since the host
 *  deploys a green main) a daily production deploy of nothing.
 *
 *  Two things follow from that. People are sorted by login rather than left in the order
 *  GitHub happened to page them in. And the file carries NO collected-at stamp: when the
 *  data last changed is already recorded, exactly and unforgeably, by the commit that
 *  changed it — a field restating that would cost the entire commit-only-on-change
 *  design in exchange for a worse copy of it. */
export function serializeSnapshot(entries) {
  const people = [...entries].sort((a, b) => a.login.localeCompare(b.login));

  return `${JSON.stringify({ people }, null, 2)}\n`;
}

// ---------------------------------------------------------------------------
// Collection. Everything below talks to GitHub and is exercised by running it,
// not by a test: what it does is fetch, and a test of a mocked fetch would only
// restate the mock.
// ---------------------------------------------------------------------------

/** The GitHub token, or a hard stop.
 *
 *  Refusing to run unauthenticated rather than falling back to it: unauthenticated
 *  REST allows 60 requests an hour and this needs roughly thirty, so a fallback would
 *  sometimes work, sometimes truncate, and never say which. */
function requireToken() {
  const token = process.env.GITHUB_TOKEN;
  if (!token) throw new Error('GITHUB_TOKEN is required — refusing to collect unauthenticated');
  return token;
}

/** One GraphQL call. Throws on transport failure and on a body carrying `errors`:
 *  GitHub answers a partially-failed query with HTTP 200 and an `errors` array beside
 *  the data, so status alone would let a half-collected page through as a whole one. */
async function graphql(token, query, variables) {
  const res = await fetch('https://api.github.com/graphql', {
    method: 'POST',
    headers: { authorization: `bearer ${token}`, 'content-type': 'application/json' },
    body: JSON.stringify({ query, variables }),
  });

  if (!res.ok) throw new Error(`github graphql ${res.status} ${await res.text()}`);

  const body = await res.json();
  if (body.errors) throw new Error(`github graphql: ${JSON.stringify(body.errors)}`);
  return body.data;
}

// The author of a pull request or an issue. `__typename` is how an automated account
// identifies itself, and `databaseId` needs a fragment per concrete type because the
// Actor interface does not carry one.
const AUTHOR_FRAGMENT = `
  author {
    __typename
    login
    avatarUrl
    ... on User { databaseId }
    ... on Bot { databaseId }
  }
`;

const MERGED_PULL_REQUESTS = `
  query ($owner: String!, $name: String!, $cursor: String) {
    repository(owner: $owner, name: $name) {
      pullRequests(states: MERGED, first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes { number title mergedAt url ${AUTHOR_FRAGMENT} }
      }
    }
  }
`;

const ISSUES = `
  query ($owner: String!, $name: String!, $cursor: String) {
    repository(owner: $owner, name: $name) {
      issues(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes { createdAt ${AUTHOR_FRAGMENT} }
      }
    }
  }
`;

/** GitHub's author shape, flattened to the one the assembly reads. */
function readAuthor(author) {
  if (!author) return null;

  return {
    login: author.login,
    id: author.databaseId ?? 0,
    avatarUrl: author.avatarUrl,
    accountType: author.__typename,
  };
}

/** Every node of a connection, paged to completion.
 *
 *  Deliberately unbounded rather than capped at some number of pages: the point of
 *  using GraphQL here is that it does not truncate the way the search API does at a
 *  thousand results, and a page cap would quietly reintroduce exactly that. */
async function pageAll(token, query, read) {
  const nodes = [];
  let cursor = null;

  for (;;) {
    // Sequential by necessity: each page's cursor comes from the one before it.
    const data = await graphql(token, query, { ...REPO, cursor });
    const connection = read(data.repository);

    nodes.push(...connection.nodes);
    if (!connection.pageInfo.hasNextPage) return nodes;
    cursor = connection.pageInfo.endCursor;
  }
}

/** The logins with admin permission on the repository.
 *
 *  Read rather than hardcoded, so a new maintainer is grouped correctly with no change
 *  to this file. Needs a token with access to the repository's collaborators, which the
 *  workflow's own token has. */
async function fetchAdmins(token) {
  const res = await fetch(
    `https://api.github.com/repos/${REPO.owner}/${REPO.name}/collaborators?per_page=100`,
    { headers: { authorization: `bearer ${token}`, accept: 'application/vnd.github+json' } },
  );

  if (!res.ok) throw new Error(`github collaborators ${res.status} ${await res.text()}`);

  const collaborators = await res.json();
  return collaborators.filter((c) => c.permissions?.admin).map((c) => c.login);
}

async function main() {
  const token = requireToken();

  const [merged, issues, admins] = await Promise.all([
    pageAll(token, MERGED_PULL_REQUESTS, (repo) => repo.pullRequests),
    pageAll(token, ISSUES, (repo) => repo.issues),
    fetchAdmins(token),
  ]);

  const entries = assembleEntries({
    pullRequests: merged.map((node) => ({ ...node, author: readAuthor(node.author) })),
    issues: issues.map((node) => ({ createdAt: node.createdAt, author: readAuthor(node.author) })),
    admins,
    excluded: EXCLUDED_LOGINS,
  });

  assertUsable(entries);

  await writeFile(SNAPSHOT_PATH, serializeSnapshot(entries));

  console.log(
    `contributors: ${entries.length} people from ${merged.length} merged pull requests and ${issues.length} issues`,
  );
}

// Only when run, never when imported by the test. A failure exits non-zero having
// written nothing, so the workflow's commit step does not run and the previously
// committed snapshot keeps serving — a partial collection must never replace a whole one.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    console.error(err.message);
    process.exit(1);
  });
}
