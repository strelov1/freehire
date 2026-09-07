// The rules the contributors pages read the committed snapshot through.
//
// Collection lives in `scripts/build-contributors.mjs`, which does I/O and nothing
// else: it pages the GitHub API and writes down everyone it found, bots included.
// Every RULE — who is shown, how they are grouped, what order they appear in — lives
// here instead, as pure functions over that file, because a rule exercised only by a
// nightly job nobody watches is a rule nobody can trust. Here it is unit-tested
// against fixtures with no network anywhere.

/** One merged pull request, as a profile page lists it. */
export interface ContributorPullRequest {
  number: number;
  title: string;
  /** ISO-8601 instant the pull request was merged. */
  mergedAt: string;
  url: string;
}

/** One person in the snapshot, exactly as the collector wrote them down.
 *
 *  `recentPullRequests` is capped by the collector rather than holding a person's
 *  whole history: the repository carries thousands of merged pull requests, almost
 *  all of them one maintainer's, and committing that daily would be a large file
 *  restating the same history. Totals stay exact; only the detail is bounded. */
export interface ContributorEntry {
  login: string;
  id: number;
  avatarUrl: string;
  /** GitHub's own account type. `Bot` is how an automated account identifies itself. */
  accountType: string;
  role: 'maintainer' | 'contributor';
  /** ISO-8601 instant of their earliest contribution, pull request or issue. */
  firstContributionAt: string;
  /** ISO-8601 instant of their latest contribution, pull request or issue. */
  lastContributionAt: string;
  mergedPullRequests: number;
  openedIssues: number;
  recentPullRequests: ContributorPullRequest[];
}

/** The committed snapshot: everyone the collector found, and when it looked. */
export interface ContributorsSnapshot {
  generatedAt: string;
  people: ContributorEntry[];
}

/** What a page renders: the two groups, each already filtered and ordered. */
export interface ContributorGroups {
  maintainers: ContributorEntry[];
  contributors: ContributorEntry[];
}

/** Whether an entry is an automated account.
 *
 *  Two signals rather than one: `accountType` is what GitHub itself reports and is the
 *  authority, and the `[bot]` login suffix catches an account reached through a path
 *  that did not carry the type. Left in, `dependabot[bot]` is the second-largest
 *  contributor to this repository — on a page about people, that is a bug. */
function isBot(entry: ContributorEntry): boolean {
  return entry.accountType === 'Bot' || entry.login.endsWith('[bot]');
}

/** Whether the entry has something the page can name as a contribution.
 *
 *  The collector reaches accounts through more than one query, so an entry can survive
 *  with nothing attached to it. A merged pull request and an opened issue both count;
 *  neither means there is nothing to show. */
function hasContributed(entry: ContributorEntry): boolean {
  return entry.mergedPullRequests > 0 || entry.openedIssues > 0;
}

/** The snapshot as the pages show it. */
export function contributorGroups(snapshot: ContributorsSnapshot): ContributorGroups {
  const people = snapshot.people.filter((entry) => !isBot(entry) && hasContributed(entry));

  return {
    maintainers: byRecency(people.filter((entry) => entry.role === 'maintainer')),
    contributors: byRecency(people.filter((entry) => entry.role !== 'maintainer')),
  };
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** What a person contributed, in the page's own words.
 *
 *  Only what they actually did: naming a kind they have none of would read as an
 *  absence — "0 issues opened" beside someone's first merged pull request is a scolding,
 *  not a summary. */
export function contributionSummary(entry: ContributorEntry): string {
  const parts = [];
  if (entry.mergedPullRequests > 0) {
    parts.push(plural(entry.mergedPullRequests, 'merged pull request', 'merged pull requests'));
  }
  if (entry.openedIssues > 0) {
    parts.push(`${plural(entry.openedIssues, 'issue', 'issues')} opened`);
  }
  return parts.join(' · ');
}

/** The one person a profile page and its social card are about, or null.
 *
 *  Resolved through the same rules the showcase renders, never against the raw
 *  snapshot: a bot, or an entry with nothing to show, must 404 rather than get a page
 *  and a shareable card the showcase itself never links to.
 *
 *  Matched case-insensitively because GitHub logins are, so a link shared with
 *  different casing reaches the page instead of a 404. */
export function findContributor(
  snapshot: ContributorsSnapshot,
  login: string,
): ContributorEntry | null {
  const wanted = login.toLowerCase();
  const { maintainers, contributors } = contributorGroups(snapshot);

  return [...maintainers, ...contributors].find((e) => e.login.toLowerCase() === wanted) ?? null;
}

/** Most recent contribution first.
 *
 *  Deliberately not by volume: this page's job is to attract the next contributor, and
 *  a volume ordering buries a newcomer the moment they arrive, behind everyone who has
 *  simply been here longer. Recency puts the newest arrival where the page's purpose
 *  needs them, and reads as "people contribute here" rather than "one person wrote
 *  this". No count is consulted. */
function byRecency(entries: ContributorEntry[]): ContributorEntry[] {
  return [...entries].sort((a, b) => b.lastContributionAt.localeCompare(a.lastContributionAt));
}
