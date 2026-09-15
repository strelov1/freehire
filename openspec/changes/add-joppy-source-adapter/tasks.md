## 1. Sitemap discovery

- [x] 1.1 Test + implement: fetch and parse `sitemap.directory.xml` into the distinct set of
      `/companies/<slug>` company slugs (dedupe `/companies/<slug>/<uuid>` posting entries down
      to their company slug too; a slug is included whether it appears as a company entry, a
      posting entry, or both).
- [x] 1.2 Test: a malformed/unreachable sitemap fails the whole crawl (board-level error, not a
      silent empty result) — this is the FIRST-page-equivalent for a directory-driven adapter.

## 2. Per-company page parsing

- [x] 2.1 Test + implement: extract the `__NEXT_DATA__` JSON blob from a company page via the
      shared `bracketSlice` helper and decode `props.pageProps.company.{name,slug}` plus the
      `jobs` array into typed structs.
- [x] 2.2 Test: a company page with an empty `jobs` array yields zero `Job`s, not an error.
- [x] 2.3 Test: a company page that fails to fetch, or whose `__NEXT_DATA__` is absent or
      unparseable, is skipped (logged) without aborting the rest of the crawl.

## 3. Posting field mapping

- [x] 3.1 Test + implement: identity fields — `ExternalID` (job `uid`), `URL`
      (`https://www.joppy.me/companies/<slug>/<uid>`), `Title`, `Company` (from the company
      page, not per-job); drop a posting missing `uid` or `title`.
- [x] 3.2 Test + implement: `Description` — sanitize the HTML body through the existing
      `sanitizeHTML` helper.
- [x] 3.3 Test + implement: work mode — `joppyWorkMode(isRemote, isHybrid, isOffice bool)
      string` per design.md's priority rule (hybrid wins; else exactly one of remote/office;
      else unset). Cover all six flag combinations observed live, including the
      `(true,true,true)` and `(true,false,true)` cases, as table-driven cases.
- [x] 3.4 Test + implement: `Location` — join `place.cities` when non-empty, else fall back to
      `place.located`, else leave empty.
- [x] 3.5 Test + implement: `Skills` — map `skills[].name` through `skilltag.Canonicalize`,
      preserving which entries are `isMandatory` for the description fold in 3.7.
- [x] 3.6 Test + implement: salary — `SalaryMin`/`SalaryMax`/`SalaryCurrency`="EUR"/
      `SalaryPeriod`="year" set only when `isSalaryPublic` is true; confirmed unset (even though
      the platform's own payload carries `salaryMin`/`salaryMax` internally) when it is false.
      Gate is `isSalaryPublic && (min != nil || max != nil)` — publishes whichever bound is
      present, matching the `edjoin`/`ukgready` convention for a one-sided range, found during
      review (a real live one-sided public salary has not been observed, but the stricter
      `min != nil && max != nil` gate would have silently dropped one).
- [x] 3.7 Test + implement: description fold for facts with no structured field — append
      human-readable sentences for `sponsorVisa`, `relocationPack`, `onlyEuCandidates` (each
      only when true/stated), and the must-have vs nice-to-have skill split, and the required
      language(s) with their stated level. Rendered as the platform's own raw "N/5" figure
      rather than mapped onto the 4-label Basic/Intermediate/Fluent/Native UI vocabulary —
      picking a bucket boundary for a 5-point scale with only 4 labels is exactly the same
      guessed-equivalence problem design.md's Decisions section rejects for CEFR, just one step
      removed, so it applies here too.

      **Found in review**: the sentences are joined with literal `"\n\n"`, which the frontend's
      `{@html}` renderer collapses to nothing (no `white-space: pre-wrap`, only real `<p>`
      elements are styled) — every fact would have rendered as one dense run-on paragraph.
      `joppyDescriptionExtras` now builds Markdown internally and converts it through the
      existing `sanitizeHTML(markdownToHTML(...))` pipeline (the same one `apple.go` and
      `getmanfred.go` already use for an assembled multi-section description) before returning,
      so a caller cannot forget the conversion step.

## 4. Registration

- [x] 4.1 Register `NewJoppy` in `sources.All` (`internal/ingest/sources/registry.go`) with
      `boardless()` + `aggregator()` markers, matching `getmanfred`'s registration shape.
- [x] 4.2 Test: `Provider()` returns `"joppy"`; a `Config.Validate` pass accepts a boardless
      `joppy` entry with an empty `Board`.

## 5. End-to-end fixture test

- [x] 5.1 Added as `TestJoppyFetch`: an inline fixture (not a saved real HTML file, to keep the
      test lean and the repo diff small) reproducing the real field shape found in live
      inspection — sitemap with company + posting entries, one company with two postings
      (`isSalaryPublic: true` and `false`, multiple `cities`), one company with zero postings,
      and one company whose page fetch fails — asserting the full `Fetch` output end to end.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go build ./...`, `go test ./...` all pass (one
      pre-existing, unrelated failure noted: `cmd/billing-sync`'s
      `TestTheStoreProviderAloneKeepsTheWorkerRunning` fails on a fresh `main` checkout before
      this change touches anything — confirmed reproducible in isolation, confined to a
      different layering block (`identity`/`billing`) `internal/ingest/sources` cannot reach).
- [x] 6.2 `go vet -tags=integration ./...` passes.
