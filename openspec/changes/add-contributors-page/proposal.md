## Why

freehire is open source, but nothing on the site names the people who built it with us.
The `/open` page carries a contributor *count* and links away to GitHub, so the one moment
a visitor is curious about the community sends them somewhere else. Eight people outside
the maintainer have landed code, and none of them has anything to point at.

This is not a trophy wall for a crowd we already have — it is the lure for the ninth
contributor. A page that gives each contributor their own URL and their own social preview
card turns recognition into something *they* share, which is the only way a page like this
reaches anyone who is not already looking at us.

## What Changes

- **New `/contributors` showcase.** Every human who has merged a pull request or opened an
  issue against the repo, with their avatar, what they contributed, and a link to their own
  page. Bots are excluded. Maintainers are shown in their own group, separate from
  contributors, so one 3000-commit history does not visually erase eight smaller ones.
- **New `/contributors/<login>` profile page.** Per person: when they first contributed,
  how many pull requests merged, how many issues opened, and the list of their actual merged
  pull requests by title and date — a page worth linking to from a CV, not just a number.
  Carries share actions for X and LinkedIn.
- **New per-contributor Open Graph card** at `/contributors/<login>/og.png`, rendered by the
  site's existing satori card pipeline, so posting the link shows the person's avatar and
  figures instead of the generic site preview.
- **New scheduled GitHub Action** that collects the data once a day using the workflow's own
  `GITHUB_TOKEN` and commits a JSON snapshot into the repo. The pages read that file. No
  request path ever calls the GitHub API, so the page cannot rate-limit, cannot fail, and
  costs nothing to serve. The Action commits only when the data actually changed.
- **`/open` links to `/contributors`.** The existing contributor count becomes the entry
  point to the new page instead of a link off-site to GitHub.
- No backend, database, migration, or cron worker. The whole change lives in `web/` plus one
  workflow file.

## Capabilities

### New Capabilities

- `contributors-page`: The public contributors showcase and per-contributor profile pages,
  the rules for who appears and how they are grouped and ordered, the per-contributor social
  card, and the daily snapshot that feeds all of it.

### Modified Capabilities

- `open-transparency-page`: The open-source stats section's contributor figure SHALL link to
  the on-site `/contributors` page rather than to the repository's contributor graph on
  GitHub.

## Impact

**Added**

- `web/src/routes/contributors/` — showcase page, `[login]/` profile page, `[login]/og.png/`
  card endpoint.
- `web/src/lib/server/og/contributor.ts` — the card markup, alongside the existing
  `company.ts` / `blog.ts` / `card.ts`, reusing the shared brand primitives in `shared.ts`.
- `web/src/lib/data/contributors.json` (+ its TypeScript type) — the committed snapshot.
- `.github/workflows/contributors.yml` — the daily collector.

**Modified**

- `web/src/routes/open/+page.svelte` — the contributor figure becomes an on-site link.
- `web/src/routes/sitemap-pages.xml/+server.ts` — the new routes become crawlable.

**Not touched**

- `web/src/lib/server/github.ts` stays as it is. It answers "how many", which the header
  badge and `/open` still need, and its 60-requests-per-hour budget is untouched by this
  change because nothing here reads GitHub at request time.

**Checks this change must satisfy**

- `actionlint` (the `workflows` CI job) lints the new workflow, including shellcheck over
  every `run:` block.
- `pnpm check:links` resolves relative Markdown links; `pnpm check:dead` (knip) gates unused
  files and dependencies across `web/`.
- The daily commit feeds the host's autodeploy poller. Committing only on a real data change
  keeps that from becoming a deploy a day.
