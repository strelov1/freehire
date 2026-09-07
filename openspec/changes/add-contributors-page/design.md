## Context

The repository today has 2410 merged pull requests and 104 issues. A first collection found
seventeen accounts behind them: one bot, the maintainer with 2236 merged pull requests, and
fifteen outside contributors whose largest history is eight. Eight of those fifteen have
never merged code and are here entirely on issues they opened — which is the measured
argument for counting issues at all, since counting only code would halve the page.

Any page built naively — a flat grid ordered by volume — renders as one enormous bar and
fifteen slivers, with a bot in second place. The shape of the data is the design constraint.

The site already has everything else this needs. `web/src/lib/server/og/` renders 1200x630
social cards through satori (`card.ts`, `company.ts`, `blog.ts`, `shared.ts` for the brand
primitives, `render.ts` for the PNG), exposed on routes shaped `…/og.png/+server.ts`.
`web/src/lib/server/github.ts` already talks to the GitHub API for the star badge and
`/open`, and documents the reason this change must not join it there: unauthenticated GitHub
REST allows 60 requests per hour per IP, and that budget is already spoken for.

`web/scripts/*.mjs` is an established home for tooling that produces a `web/` asset
(`gen-og.mjs`, `og-smoke.mjs`, `gen-api-docs.mjs`), is already a knip entry point, and —
unlike root `scripts/` — sits beside a test runner. `web/` runs vitest.

## Goals / Non-Goals

**Goals:**

- A `/contributors` page whose first impression is "people contribute here, and recently",
  not "one person wrote this".
- A `/contributors/<login>` page a contributor would willingly link to from a CV — their
  actual pull requests by title, not just a number.
- A social preview card per contributor, so the shared link shows their face.
- Zero GitHub API calls on any request path, and zero new backend, database, or worker
  surface.

**Non-Goals:**

- **No medals or tiers.** Novu's bronze/silver/gold is a good idea at fifty contributors and
  premature at eight; the profile page already shows the counts a tier would be derived
  from. The seam is the snapshot's per-person figures — a tier can be computed from them
  later without recollecting anything.
- **No manually curated contribution types.** Docs, design, and triage credit (the
  `all-contributors` model) needs a hand-maintained list, and a hand-maintained list is
  exactly the thing that silently omits people. Merged pull requests and opened issues are
  both machine-readable and both count.
- **No live freshness.** A day-old snapshot is correct enough for a page about people.
- No leaderboard, no ranking, no points.

## Decisions

### The snapshot is a committed JSON file, collected by a scheduled Action

**Chosen:** `.github/workflows/contributors.yml` runs daily, executes
`node web/scripts/build-contributors.mjs`, and commits `web/src/lib/data/contributors.json` when
it differs. The workflow's own `GITHUB_TOKEN` carries a 5000-requests-per-hour budget and
`contents: write` permission for the commit.

**Alternatives considered:**

- *Fetch live at request time, memoized like `githubStats`.* Rejected: per-person pull-request
  lists cannot be assembled inside 60 requests per hour, the memo dies on every restart, and
  blue/green means two processes each spending the budget separately.
- *A `cmd/sync-contributors` worker writing to Postgres.* Idiomatic for this repo and
  genuinely more powerful, but it buys freshness nobody needs at the cost of a migration,
  sqlc regeneration, a Go endpoint, and a systemd unit that `release.sh` does not deploy —
  the unit would have to be copied to the host by hand. Not worth it for eleven rows that
  change monthly.

### Collection uses the GraphQL API, not the Search API

The Search API caps a query at 1000 results and 2408 merged pull requests exceed it, so
paging merged PRs through search silently truncates. GraphQL
`repository.pullRequests(states: MERGED)` pages to completion at 100 per call — about 25
calls — and returns author, number, title, and `mergedAt` in one shot. Issues (104) page in
two calls. Maintainership comes from the REST collaborators endpoint, filtered to
`permissions.admin`; the Action's token has the access to read it.

### The snapshot keeps counts for everyone and details for the recent twenty

Storing all 2408 pull requests would put a ~300 KB file through a daily commit, 99% of it
one person's history. Each person's entry therefore carries their totals plus their twenty
most recent merged pull requests. Twenty is what a profile page can show without becoming a
scroll, and the rule is uniform — no special case for the maintainer, so nothing about the
file changes shape when a second maintainer appears.

### Rules live in `web/src/lib/contributors.ts`, collection lives in the script

The script does I/O: paginate, map fields, cap at twenty, write. Its assembly is unit-tested
from `web/scripts/build-contributors.test.mjs`, which cost one glob in `web/vitest.config.ts`
(the include list covered only `src/**/*.test.ts`). It writes everyone it
found, bots included.

Everything the spec calls a rule — excluding bots, splitting maintainers from contributors,
ordering by most recent contribution — lives in `web/src/lib/contributors.ts` as pure
functions over the snapshot, unit-tested with vitest against fixtures. This is what makes
the requirements testable: a test can assert that a bot never survives the filter and that a
one-week-old newcomer outranks a year-old eight-commit history, with no network anywhere.

The alternative — putting the rules in the script — would leave them exercised only by a
scheduled job nobody watches, in a file the root workspace has no test runner for.

### Ordering is by most recent contribution, descending

Sourcerer's widget separates "new" and "trending" from "top" for exactly this reason: on a
volume ordering, a newcomer is buried the moment they arrive, which is the opposite of what
this page is for. One recency ordering achieves the same effect with no categories to
explain. The maintainer group sits apart, so the maintainer's recency does not occupy the
first slot the newcomer needs.

### Opt-out is an explicit exclusion list in the script

A person may not want their name on a marketing page. `web/scripts/build-contributors.mjs`
carries an `EXCLUDED_LOGINS` constant, empty at first, honored on request. This is a
hand-written list, which the repo generally distrusts — but the hazard of a hand-written
list is that it hides people who should be there, and an exclusion list has the opposite
failure mode: forgetting to add someone shows them, which is the state they were already in.

## Risks / Trade-offs

- **A daily commit triggers the host's autodeploy poller** → The workflow commits only when
  the collected data differs. With eleven contributors that is a handful of commits a month,
  each of them a real change worth deploying.
- **A partial collection would silently shrink the published list** → The script fails the
  run on any incomplete page rather than writing what it has; the workflow's commit step
  never runs, and the previous snapshot keeps serving.
- **A contributor deletes their GitHub account** → Their avatar 404s. The OG card already
  degrades to the shared monogram, and the profile page shows the same fallback; the entry
  disappears on the next collection.
- **satori is strict about layout** (flexbox only, `display: flex` on any multi-child
  element, per the note in `card.ts`) → The contributor card reuses `shared.ts` primitives
  and is covered by the same render smoke test the existing cards use.
- **The `[login]` route can be requested with arbitrary input** → Lookup is an exact match
  against the snapshot's logins; anything absent is a 404 on both the page and the card, and
  every interpolated value is escaped before reaching the card markup.
- **knip gates unused files across `web/`** → The snapshot JSON and the rules module are both
  imported by the pages, and `scripts/*.mjs` is already an entry point, so nothing needs a
  config exemption. Worth re-checking with `pnpm check:dead` before the PR.
- **`actionlint` shellchecks every `run:` block** → The workflow keeps its `run:` blocks to
  single commands and puts logic in the `.mjs` script.

## Migration Plan

Additive and reversible. The first snapshot is committed by hand (by running the script
locally with a personal token) so the pages have data on the deploy that introduces them;
the scheduled workflow takes over from the next run. Rollback is deleting the routes and the
workflow — nothing else reads the snapshot, and `web/src/lib/server/github.ts` is untouched,
so `/open` and the star badge behave exactly as before either way.

## Open Questions

None blocking. Two to revisit once the page is live and has been shared a few times: whether
the twenty-pull-request cap is the right depth for a profile, and whether tiers become worth
adding once the contributor count reaches a size where they would mean something.
