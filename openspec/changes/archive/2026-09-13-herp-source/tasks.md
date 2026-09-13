## 1. Link matching

- [x] 1.1 Write a host+path-shape matcher for a HERP job link (`/v1/<board>/<jobID>`, single
      segment, excluding `/apply`), matching by resolved host/path, never substring
- [x] 1.2 Write the matching predicate for a requisition-group link
      (`/v1/<board>/requisition-groups/<uuid>`)
- [x] 1.3 Confirm a share-widget link whose host is not `herp.careers` is never matched, even
      when its query string embeds a real HERP URL

## 2. Listing discovery

- [x] 2.1 Fetch the board's listing page and collect direct job links via `jobLinks`
- [x] 2.2 Collect requisition-group links, fetch each one, and add its job links to the same
      set, deduplicated across all pages
- [x] 2.3 A failure fetching the listing page, or any requisition-group page, fails the whole
      `Fetch` call

## 3. Job detail

- [x] 3.1 Fetch a job link and decode its `application/ld+json` `JobPosting` block via the
      shared `ldJobPosting` helper (title, description, datePosted, jobLocation.address)
- [x] 3.2 Map to the normalized `Job` shape: `external_id` = the link's final path segment,
      sanitized HTML description, `posted_at` parsed from `datePosted`
- [x] 3.3 A failed job detail fetch marks only that posting Unreadable (`unreadableDetail`),
      never the whole board

## 4. Registration

- [x] 4.1 Implement `fullBoardListing()` and register `herp` in `sources.All`
- [x] 4.2 Add the `herp.careers` → `herp` `atsBoards` entry (`path` mode) and the
      `reservedSegments["herp.careers"] = []string{"v1"}` entry, plus a `TestRecognize` case
- [x] 4.3 Add a `TestRecognize` case confirming a bare `/v1` (no board) is declined

## 5. Verification

- [x] 5.1 Run the full adapter test suite plus `atsboard`, `boardresolve`, `linksource`,
      `contribution`, `atsdetect`
- [x] 5.2 Confirm `herp` needs no `cmd/harvest-boards` prober entry — verify it resolves via
      `adapterProber` fallback (added to the existing
      `TestProberForFallsBackToTheProvidersAdapter` table)

## 6. Review fix

- [x] 6.1 Code review (live-verified against real herp.careers pages) found the platform's own
      `/v1/<board>/top` landing-page link passes the job-link shape check on boards that have
      one, permanently withholding that board's stale-job close. Excluded the literal `top`
      segment (`herpNonJobSegments`), with a regression test reproducing the live case
