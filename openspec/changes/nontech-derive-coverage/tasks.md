## 1. Category dictionary — administration block

- [x] 1.1 Add classify test cases for `admin assistant`, `administrative
      coordinator`, `administrative specialist`, `front desk`, `virtual
      assistant` → `administration` (including a grade-prefixed variant, e.g.
      "Senior Admin Assistant")
- [x] 1.2 Add classify test cases proving the bare-`assistant` exclusion holds:
      "Assistant Controller", "Assistant Superintendent", "Assistant
      Director", "Maintenance Assistant", "Clinic Assistant", "Laboratory
      Assistant" stay unresolved
- [x] 1.3 Add the five ADMINISTRATION aliases to
      `internal/dict/classify/dictionaries.go`; do not add a bare `assistant`
      entry
- [x] 1.4 `go test ./internal/dict/classify/...` green

## 2. Category dictionary — legal/immigration block

- [x] 2.1 Add classify test cases for `immigration paralegal`, `immigration
      specialist`, `immigration assistant`, `immigration consultant`,
      `immigration case manager` → `legal`
- [x] 2.2 Add a test proving "Immigration Case Manager" resolves to `legal`
      and is not stolen by the terminal `manager` → `management` fall-through
- [x] 2.3 Add the five immigration aliases to the LEGAL block in
      `internal/dict/classify/dictionaries.go`, ordered ahead of the bare
      `manager` fall-through
- [x] 2.4 `go test ./internal/dict/classify/...` green

## 3. Skill dictionary — support, scheduling and legal-practice tooling

- [x] 3.1 Add skilltag test cases: a description naming Freshdesk, Calendly,
      Clio, USCIS, "Form I-129" and "Form I-130" resolves each to its
      canonical (`freshdesk`, `calendly`, `clio`, `uscis`, `i-129`, `i-130`)
- [x] 3.2 Add the six alias→canonical entries to
      `internal/dict/skilltag/dictionaries.go`
- [x] 3.3 Add each canonical's reader-facing label to
      `internal/dict/skilltag/labels.go`
- [x] 3.4 Add each canonical's one-sentence definition to
      `internal/dict/skilltag/descriptions.tsv`
- [x] 3.5 `go test ./internal/dict/skilltag/...` green (description-coverage
      test included)

## 4. Work-mode phrases

- [x] 4.1 Add location/workmode test cases: a description whose only remote
      signal is "work from home", "work-from-home", "home-based", "home
      based", "telecommute" or "virtual position" derives `work_mode =
      remote` when no structured or location signal is present
- [x] 4.2 Not a new test: `TestDerive_StructuredWorkModeWins` and
      `TestDerive_LocationWorkModeBeatsDescription`
      (`internal/job/jobderive/jobderive_test.go`) already prove a structured
      signal is never overwritten by `WorkModeFromDescription`'s output,
      generically over any matched phrase — a new test with a new phrase
      would exercise the identical code path, so none was added
- [x] 4.3 Add the six phrases to `descriptionWorkModePhrases` in
      `internal/dict/location/workmode.go`
- [ ] 4.4 **BLOCKED — needs prod access this session does not have.** Pull a
      sample of live descriptions containing "work from home" (via
      Meilisearch/prod query, read-only) and classify each occurrence as a
      work-arrangement statement vs. a bounded benefit/perk mention — decide
      from the sample whether "work from home" needs a `travelPerkPhrases`
      guard, mirroring how "work from anywhere" was decided (freehire#2696).
      The local dev stack (`hire-db-1`/`hire-meilisearch-1`) does not hold
      the real crawled catalogue, so it cannot substitute for this sample.
      Until this runs, "work from home" resolves `remote` unconditionally,
      same as every other phrase in the list — see the deterministic-facets
      spec delta's note and design.md's Migration Plan step 2.
- [ ] 4.5 **BLOCKED on 4.4.** If the sample supports it, add "work from
      home" to `travelPerkPhrases` with a test proving a benefit-list
      sentence containing it does not resolve `work_mode` to `remote`
- [x] 4.6 `go test ./internal/dict/location/...` green

## 5. Whole-package verification

- [x] 5.1 `go build ./...`
- [x] 5.2 `go vet ./...`
- [x] 5.3 `go test ./...` — green except `cmd/billing-sync`'s
      `TestTheStoreProviderAloneKeepsTheWorkerRunning`, confirmed pre-existing
      and unrelated (deterministic failure with zero diff overlap; this
      change touches no file under `cmd/billing-sync` or
      `internal/identity/billing`)
- [x] 5.4 `gofmt -l .` prints nothing for changed files
- [x] 5.5 (not originally listed) `go generate ./internal/dict/skillvec/` —
      required once the six new skilltag canonicals existed;
      `TestRegistryCoversEveryCanonicalSkill` is what caught the gap

## 5a. Code review fixes

- [x] 5a.1 Branch was accidentally based on an in-progress, unrelated local
      branch (`tier-badge-visibility`) instead of `origin/main` — rebased
      `worktree-nontech-derive-coverage` onto `origin/main` to drop that
      commit from the diff
- [x] 5a.2 `clio` gated in `ambiguousWords` (collides with a common first
      name, the Clio Awards, and the Renault Clio — the same class of
      collision `maya`/`lottie`/`houdini` are already gated for); added
      `TestParse_ClioNeedsCorroboration` and updated
      `skill-tag-matching/spec.md`'s scenario accordingly
- [x] 5a.3 Corrected the `immigration paralegal` alias comment — it is
      reachable only through the pre-existing bare `paralegal` entry, not
      load-bearing like its four siblings
- [x] 5a.4 Alphabetized the new `wordAliases` block in
      `internal/dict/skilltag/dictionaries.go`

## 6. Rollout & verification (operational — run against prod after merge, not part of this PR)

- [ ] 6.1 Run `cmd/backfill-derive` with `BACKFILL_CONCURRENCY=2` or `3`
      (measured degrading prod at 6; ~15h) over the full catalogue
- [ ] 6.2 Follow with a full `make reindex` (no incremental path — none of
      `category`/`is_tech`/`work_mode`/`skills` are in `content_hash`); do not
      stack it on top of an in-flight `search-drain` or `reindex-companies`
      run
- [ ] 6.3 Re-run the three verification queries from
      `docs/superpowers/specs/2026-09-19-nontech-derive-coverage-design.md`
      (segment postings excluded by `search.CategoryUnresolved`; segment
      postings now flagged `remote`; trap-list spot-check) and compare
      against its recorded baseline
- [ ] 6.4 Delete the "Owed right now" section of
      `internal/dict/classify/AGENTS.md` once the backfill completes (this
      run clears that #2847/#2849 debt as a side effect)
- [ ] 6.5 Update the design record's status line from "design approved, not
      implemented" once rollout is verified
