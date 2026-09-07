## 1. Snapshot shape and rules

- [x] 1.1 Define the snapshot's TypeScript type in `web/src/lib/contributors.ts`: per person the login, numeric id, avatar URL, role (`maintainer` / `contributor`), account type, first-contribution date, last-contribution date, merged-PR total, opened-issue total, and up to twenty recent merged pull requests (number, title, mergedAt, url); plus the snapshot's own generated-at stamp.
- [x] 1.2 Write failing vitest cases in `web/src/lib/contributors.test.ts` for bot exclusion: an entry whose account type is `Bot` and an entry whose login ends in `[bot]` are both absent from the returned groups and from every count derived from them.
- [x] 1.3 Implement the bot filter in `web/src/lib/contributors.ts` to make 1.2 pass.
- [x] 1.4 Write failing vitest cases for the maintainer split: a `maintainer`-role entry appears in the maintainer group and never in the contributor group.
- [x] 1.5 Implement the maintainer split to make 1.4 pass.
- [x] 1.6 Write failing vitest cases for ordering: given one contributor with eight merged PRs a year ago and one with a single merged PR last week, the recent one is first; ordering never consults any count.
- [x] 1.7 Implement recency ordering to make 1.6 pass.
- [x] 1.8 Write a failing vitest case for an issues-only contributor (zero merged PRs) and make the rules module return them with an empty pull-request list rather than dropping or throwing.
- [x] 1.9 Add a checked-in fixture snapshot for the tests, and commit a real first `web/src/lib/data/contributors.json` produced by the collector in group 2.

## 2. The collector script

- [x] 2.1 Create `web/scripts/build-contributors.mjs` that pages `repository.pullRequests(states: MERGED)` through the GitHub GraphQL API at 100 per call, collecting author login, id, avatar, PR number, title, and mergedAt.
- [x] 2.2 Extend it to page repository issues the same way, collecting author and creation date.
- [x] 2.3 Read the REST collaborators endpoint and mark logins with `permissions.admin` as role `maintainer`, everyone else `contributor` — no hardcoded logins.
- [x] 2.4 Assemble per-person entries: totals for everything, details capped at the twenty most recent merged pull requests, first and last contribution dates spanning both PRs and issues.
- [x] 2.5 Honor an `EXCLUDED_LOGINS` constant (empty initially) so a person can be removed on request.
- [x] 2.6 Fail the run with a non-zero exit on any incomplete page or API error, writing nothing — verify by pointing the script at a token-less environment and confirming it exits non-zero and leaves the existing file untouched.
- [x] 2.7 Write the assembled snapshot to `web/src/lib/data/contributors.json` with stable key ordering, so an unchanged collection produces a byte-identical file.
- [x] 2.8 Run it against the live repository with a personal token and commit the resulting snapshot (satisfies 1.9).

## 3. The showcase page

- [x] 3.1 Add `web/src/routes/contributors/+page.server.ts` loading the snapshot through the rules module, and `+page.svelte` rendering the maintainer group and the contributor group separately.
- [x] 3.2 Render each contributor entry with avatar, login, a summary of their contribution, and a link to `/contributors/<login>` on this site.
- [x] 3.3 Add the page's own metadata and the site-wide OG card, following the existing pages' pattern.
- [x] 3.4 Verify the page renders correctly with the real snapshot: no bots present, maintainer separated, newest contributor first.

## 4. The profile page

- [x] 4.1 Add `web/src/routes/contributors/[login]/+page.server.ts` resolving the login against the snapshot by exact match, returning 404 when absent.
- [x] 4.2 Add `+page.svelte` showing avatar, GitHub profile link, first-contribution date, merged-PR count, opened-issue count, and the recent merged pull requests with title, number, merge date, and link.
- [x] 4.3 Render the empty-pull-request case (an issues-only contributor) without an empty section that reads as broken.
- [x] 4.4 Add X and LinkedIn share actions carrying this page's own URL.
- [x] 4.5 Verify an unknown login returns 404 and a known one renders their pull requests.

## 5. The per-contributor OG card

- [x] 5.1 Add `web/src/lib/server/og/contributor.ts` building the card markup from a contributor entry, reusing the brand primitives in `shared.ts` and escaping every interpolated value; keep to satori's flexbox-only constraint.
- [x] 5.2 Add `web/src/routes/contributors/[login]/og.png/+server.ts` rendering it, mirroring the company card endpoint's structure and cache headers.
- [x] 5.3 Degrade to the shared monogram when the avatar cannot be fetched, and confirm the response is still 200 with a valid PNG.
- [x] 5.4 Return 404 with no image for a login absent from the snapshot.
- [x] 5.5 Point the profile page's Open Graph and Twitter image metadata at its own card.
- [x] 5.6 Extend the existing OG render smoke test to cover the contributor card.

## 6. Wiring and discoverability

- [x] 6.1 Make the `/open` page's contributor count link to `/contributors` instead of GitHub, keeping the existing fallback when the GitHub leg degraded.
- [x] 6.2 Add `/contributors` and every `/contributors/<login>` to `web/src/routes/sitemap-pages.xml/+server.ts`.
- [x] 6.3 Add a `/contributors` link to the site chrome so the page is reachable without knowing the URL.

## 7. The scheduled workflow

- [x] 7.1 Add `.github/workflows/contributors.yml` running daily on a cron with `contents: write`, executing `node web/scripts/build-contributors.mjs`.
- [x] 7.2 Commit the result only when `git diff --quiet` reports a change, so an unchanged collection produces no commit and no deployment.
- [x] 7.3 Keep every `run:` block to a single command so actionlint's shellcheck pass stays clean; verify with actionlint locally.
- [ ] 7.4 Trigger the workflow manually (`workflow_dispatch`) once and confirm it produces no commit against the snapshot committed in 2.8.

## 8. Verification

- [x] 8.1 `pnpm --dir web lint`, `pnpm --dir web test`, and `pnpm --dir web build` all pass.
- [x] 8.2 `pnpm check:dead` reports no new findings for the added files.
- [x] 8.3 `pnpm check:links` passes.
- [x] 8.4 Open `/contributors`, one profile, and its `og.png` in a real browser and confirm the card previews correctly.
