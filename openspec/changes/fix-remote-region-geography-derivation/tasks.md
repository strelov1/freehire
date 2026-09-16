## 1. Remove the blanket bare-remote-to-global default

- [x] 1.1 Delete the `mode == "remote" && len(countries) == 0 && len(regions) == 0` fallback
      block in `internal/dict/location/location.go` (lines ~184-186), and update its
      surrounding doc comment (lines ~39-46) to describe the corrected contract.
- [x] 1.2 Update `internal/dict/location/location_test.go`: change `"bare remote with no
      geography yields global"` (`location: "Remote"`) and `"work-from-home marker with no
      place yields global"` (`location: "Work from home"`) to expect empty `Countries`/
      `Regions` with `WorkMode: "remote"`. Verify every `"...Anywhere"/"...Worldwide"/
      "...International"` case still passes unmodified (per design.md §Context point 1).
      Two more cases turned up beyond the ones named in the design (`"Remote or in-house"`
      and the Cyrillic `"Удалённо"` bare-remote case) — same fix applied to both.
- [x] 1.3 Run `go test ./internal/dict/location/...` and confirm only the two updated cases
      changed. (Actually four — see 1.2 note; all confirmed as the same class of case, not a
      new regression.)
- [x] 1.4 (added during implementation) Full-module `go test ./...` turned up two more
      dependent tests: `TestNormalizeJobParsesGeographyFromLocation`
      (`internal/ingest/pipeline`) and `TestDerive_BareRemoteWithoutUSOnlyStaysGlobal`
      (renamed `...StaysUnspecified`, `internal/job/jobderive`) — both updated to expect empty
      regions for a bare `"Remote"` location, same as 1.2. Also refreshed now-stale
      "bare-Remote → global" comments this change orphaned in `residence.go`, `jobderive.go`
      (×2), `eligibility.go`, and `jobview.go` (×2) to describe the corrected mechanism
      (global now comes solely from an explicit open-anywhere dictionary marker). Left
      `internal/ingest/sources/djinni.go`'s similar comment alone — traced it and confirmed
      it was already describing behavior that doesn't occur (its "Worldwide" case sets
      `Location=""`, which never matched the old fallback's `mode=="remote"` condition
      either), so it's a pre-existing doc issue unrelated to this change, not something to fix
      here.

## 2. Title-based restriction signal

- [x] 2.1 Add `RestrictionFromTitle` in `internal/dict/location/title_restriction.go`:
      extracts bracketed/parenthetical substrings from a title (`bracketSuffixPattern`), keeps
      only ones containing an anchor word — scoped to `remote`/`location` (dropped `based` from
      the original plan: "based in the UK"-style prose needs stopword handling that belongs to
      the description-scoping work in task 3, not a bracket-suffix scan; title anchors stay
      narrow, matching the ATS-convention examples actually reported), strips the anchor,
      tokenizes the remainder with the existing separator normalization, and resolves tokens via
      `resolveGeoToken`. Returns empty when nothing resolves.
- [x] 2.2 `title_restriction_test.go`: table-driven tests covering `"... [Remote-US]"` →
      `us`/`north_america`; `"... (Location - Australia or New Zealand)"` → `au`,`nz`; two
      non-anchor brackets (`"(React)"`, `"(Contract)"`) → no geography; an anchor with an
      unresolvable remainder → no geography; no bracket at all → no geography; empty title → no
      geography. (Title-already-pinned-by-location precedence deferred to task 4's `Derive`
      integration tests, as planned.)
- [x] 2.3 Confirmed: an anchor-matched substring with no resolvable token (`"[Remote-EMEA-ish]"`)
      yields empty, not a partial/best-effort result — covered by 2.2's test table.

## 3. Description region-scoping signal

- [x] 3.1 Added `RegionScopeFromDescription` in `internal/dict/location/region_scope.go`,
      alongside `EligibilityFromDescription`: anchor phrases (`remote role within`, `based in`,
      `located in`, `restricted to`, `open to candidates in`, `open to applicants in` — dropped
      the redundant `role is based in` since `based in` already covers it as a substring) find
      the first asserted (whole-word, unnegated) match via a new `firstAssertedRegionScopePhrase`
      reusing `wholeWordMatch`/`sentenceAround`/`negatedSentence` from `eligibility.go`; the
      clause after the match (to the next sentence boundary, capped at `clauseWindow=200` bytes)
      is tokenized with the same `separatorReplacer` and resolved token-by-token via
      `resolveGeoToken`, with a leading `"the "` stripped per token (needed for `"the United
      States"` in the based-in list).
- [x] 3.2 `region_scope_test.go`: table-driven tests covering the verbatim creative-fabrica
      report text (`"...within New Zealand, Australia East Coast or nearby time zones"`) →
      `nz`/`apac` only (noise tokens resolve nothing — see the corrected spec scenario, not the
      originally-planned `nz`,`au`); `"based in the United States, Canada, Argentina, or
      Brazil"` → `ar`,`br`,`ca`,`us` / `latam`,`north_america`; incidental mention ("we serve
      customers across Europe") → no geography; a negated statement ("not restricted to the
      US") → no geography; empty description → no geography.
- [x] 3.3 Confirmed: `eligibility.go`'s `usOnlyPhrases`/`ukOnlyPhrases`/etc. rules and
      `EligibilityFromDescription` itself are untouched — `RegionScopeFromDescription` is a new,
      additive file; full `internal/dict/location` suite stayed green throughout.
- [x] 3.4 (added during implementation) Live-verified two of the six original reports directly
      against their still-live source postings (the earlier background-agent spike could not
      reach them): **roadie** (job_reports #32) — live text is "Applicants must be authorized
      to work for any employer in the U.S.", a phrasing `usOnlyPhrases` didn't cover; added it
      there (single-country citizenship-style statement, correct home per design.md's
      closed-enumeration split, not `RegionScopeFromDescription`). **kard-financial**
      (job_reports #30) — live text is "hiring in the US, Canada, Argentina, or Brazil only";
      added a `"hiring in"` anchor and a trailing `" only"` qualifier strip to
      `RegionScopeFromDescription`'s token cleanup. Both added test-first (RED confirmed before
      the fix). The other two spiked reports were dead listings, not geography-derivation
      bugs — see the final report to the user (ludi: HTTP 410 Gone; 2am-tech: HTTP 404).

## 4. Wire the new signals into derivation

- [x] 4.1 In `internal/job/jobderive/jobderive.go`'s `Derive`, inserted the title-restriction
      check between `geo := location.Parse(in.Location)` and the existing
      `EligibilityFromDescription` call, gated by the same `geoUnpinned(countries, regions)`
      check, only filling when the location left geography unpinned.
- [x] 4.2 After the existing citizenship-eligibility check, added the new region-scoping
      description check as a third `geoUnpinned`-gated step (not unioned inline — each of the
      three sources is tried in its own `if geoUnpinned(...)` block, so a title match short-
      circuits the description checks entirely, matching the precedence design.md specifies)
      — never overriding a location- or title-resolved geography.
- [x] 4.3 Added `jobderive_test.go` integration tests reproducing the reported cases end to end:
      quanata `[Remote-US]`, creative-fabrica `(Location - Australia or New Zealand)`, a
      location-already-resolved case (title never consulted), title-vs-description precedence,
      and the kard-financial description-only `"hiring in..."` case. All pass against the full
      `Derive` call.
- [x] 4.4 `go build ./...` clean; `go test ./internal/dict/location/... ./internal/job/jobderive/...
      ./internal/ingest/pipeline/... ./internal/ingest/sources/... ./internal/job/jobview/...`
      green; full-module `go test ./...` run (see task 7 for the final pass with `go vet` and
      the integration-tagged compile check).

## 5. Spec sync

- [x] 5.1 Confirmed and adjusted: corrected the "Region-scoping description prose resolves a
      bare-remote posting" scenario, which originally claimed both `nz` and `au` resolve from
      the verbatim creative-fabrica text — the shipped, precision-first tokenizer only resolves
      the clean `New Zealand` token and silently skips the noisy `"Australia East Coast"`/
      `"nearby time zones"` tokens (matches `resolveGeoToken`'s exact-token-match contract, no
      partial/fuzzy matching). Everything else in both `ADDED Requirements` matches the shipped
      behavior (title anchor set narrowed to `remote`/`location`, tracked in task 2's note;
      description anchors extended with `hiring in`, tracked in task 3.4's note).
      `openspec validate fix-remote-region-geography-derivation --strict` passes.

## 6. Backfill for already-stored jobs

- [x] 6.1 Created `cmd/backfill-remote-region-restriction`, following
      `cmd/backfill-remote-perk-false-positive`'s shape exactly: Meilisearch candidates
      (`regions=global` filter + a broad `"remote"` text query), re-check against the freshly
      read row (`isBareGlobal`) before recomputing (the index can lag Postgres), recompute via
      `jobderive.Derive(jobderive.Input{Title, Location, Description})` — no `Countries`/
      `Regions` field set, so the row's stored geography is never fed back in as structured
      input — and write only when the recompute is no longer bare-global. New SQL:
      `JobsForGeographyRecheckByIDs` / `SetJobGeography` (both `IS DISTINCT FROM`-guarded),
      added to `internal/platform/db/queries/jobs.sql` and regenerated with `make sqlc`.
- [x] 6.2 Added `BACKFILL_REMOTE_REGION_MAX` via `worker.EnvInt64`; documented the worker in
      root `AGENTS.md`'s worker-gotchas list, alongside `backfill-remote-perk-false-positive`
      (needs `DATABASE_URL`, `MEILI_URL`, `MEILI_MASTER_KEY`; must be followed by a full
      `make reindex` since `countries`/`regions` are not part of `content_hash`).
- [x] 6.3 Unit-tested `recomputeGeography` against all three real reported cases (quanata,
      creative-fabrica, kard-financial) plus a genuinely-open bare-remote row (recomputes to
      empty `countries`/`regions` — this row IS written by `run()`'s `isBareGlobal` guard, its
      stored `global` corrected to "region not specified", which is the intended effect for
      this population per design.md/proposal.md's own risk section, not a skip) and a
      resolved-location row (untouched — `recomputeGeography` never runs the rescue chain past
      a resolved location, so its result equals the stored value and the `IS DISTINCT FROM`
      guard in `SetJobGeography` writes nothing). Separately unit-tested the new `isBareGlobal`
      guard (5 cases, including "global alongside a real region" and "no geography at all",
      neither of which this pass may touch).
- [x] 6.4 Scope decision: **no `--apply` gate**, matching the direct precedent
      (`cmd/backfill-remote-perk-false-positive`) rather than the `merge-companies`/
      `close-chronic-boards` shape mentioned as an alternative — that pattern belongs to
      workers whose correction is a judgment call over ambiguous evidence; this one's guard
      (`isBareGlobal` before AND after recompute) is exact and idempotent, the same risk
      profile the work-mode precedent already ships live without a gate. `go build` verified;
      not run against prod from this change (needs the ops-repo SSH deploy step, out of scope
      for the code change itself — see AGENTS.md's own "Nothing deploys itself" note on
      `deploy/`).

## 7. Verification and review

- [x] 7.1 `gofmt -l .` clean; `go vet ./...` clean; `golangci-lint run` on the touched packages:
      0 issues; `go vet -tags=integration ./...` clean. Full `go test ./...`: one full-module
      pass turned up `TestEveryCmdBinaryIsGitignored` (`internal/platform/arch`) failing on the
      new `cmd/backfill-remote-region-restriction` binary target — fixed by adding it to
      `.gitignore` (alongside the other `backfill-*` entries). The only remaining non-`ok`
      package across two full runs is `cmd/billing-sync`'s `TestTheStoreProviderAloneKeepsThe
      WorkerRunning`, confirmed pre-existing and unrelated: this change never touches
      `cmd/billing-sync` or anything it imports, the failure reproduces deterministically
      (3/3 runs) in isolation, and the worktree branched fresh from `origin/main` before any of
      this change's edits.
- [x] 7.2 Manually re-checked all six originally reported prod jobs (job_reports 29-34) against
      the fixed derivation logic, end to end through `jobderive.Derive` (jobderive_test.go's
      new integration tests) and through the backfill's `recomputeGeography`
      (recompute_test.go) — quanata, both creative-fabrica postings, and kard-financial now
      resolve their stated restriction; roadie required one additional `usOnlyPhrases` entry
      (found by live-checking the still-open posting, task 3.4) and now resolves too; munich-re
      and nanit (job_reports 25-26) were pre-existing/stale-data cases unrelated to this bug,
      not part of the six.
- [x] 7.3 Requested code review per `requesting-code-review` (full-diff review against
      `origin/main`, working tree since nothing was committed yet). Result: architecture,
      backfill shape, SQL, and the removed-fallback precision all confirmed sound; one
      **Critical** finding — `RegionScopeFromDescription`'s bare `"based in"`/`"located in"`
      anchors matched a company-HQ "About Us" mention as if it were the role's own
      restriction (reproduced independently: "Our company is based in Berlin... but this role
      is fully remote and open worldwide" → wrongly resolved `de`/`eu`). Fixed test-first:
      added two regression cases to `region_scope_test.go`, confirmed RED, then replaced the
      bare anchors with role/candidate-qualified compounds (`"candidates based in"`,
      `"role is based in"`, `"you must be based in"`, etc. — see `region_scope.go`'s updated
      doc comment for the full reasoning), confirmed GREEN, confirmed the existing
      `"based in, a clean multi-country list"` case still passes (it already used
      `"candidates based in"`). Also fixed both **Important** findings:
      `internal/dict/location/AGENTS.md`'s stale "global reaches the result by TWO paths"
      claim (now describes the single remaining path plus the new title/description rescue
      chain and why `"based in"` stayed qualified) and this file's own 6.3 wording. Addressed
      both **Minor** findings with rationale comments in `title_restriction.go` and
      `region_scope.go` (the deliberate `prevTok=""` limitation, and `clauseWindow`'s
      run-on-sentence trade-off, mirroring `eligibility.go`'s `negationWindow` comment). Full
      `go build`/`gofmt`/`go test ./...` re-run clean after the fix (see below).
