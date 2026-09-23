## 1. Negative cases first

- [x] 1.1 Add the corpus lookalikes to `internal/dict/classify/tech_test.go` as
  cases that MUST NOT be flagged: `Business Developer`, `Senior Business Developer`,
  `Job Developer`, `Product Developer`, `Project Developer`, `Project Engr II`,
  `Field Service Engr II`. Written before the terms, so a term that over-reaches
  fails the moment it is added rather than after review.

## 2. Terms

- [x] 2.1 Vendor platforms: the `<platform> developer` forms the corpus carries.
  Test first: each resolves as technical.
- [x] 2.2 Level-qualified developer: `senior developer`, `lead developer`,
  `junior developer`. Test first: they resolve, and the negative cases from 1.1
  still do not.
- [x] 2.3 `IT`-anchored roles, always two words — never bare `it`, which is the
  English pronoun once lowercased. Test first: a title with `it` as a pronoun
  ("Make It Happen Coordinator") stays unrecognised.
- [x] 2.4 Surface forms: `software engr` and the plurals of terms already listed.
  Test first: `Software Engr II` and `Software Engineers` resolve, `Project Engr II`
  does not.

## 3. Verify against the corpus that found the gaps

- [x] 3.1 Run the new detector over the 160-title prod corpus and record what it now
  recognises and what it still does not, so the change's effect is a measurement and
  not a claim. Anything newly recognised that should NOT be gets a negative test and
  a term removed.
  **Result:** three probe-and-curate passes. Coverage of the IT corpus went 49% → 75%
  → **82%** (3,573 of 4,366 postings, 125 titles). Against the 600 commonest
  unrecognised titles overall (167,460 postings — seamstresses, chambermaids, German
  retail apprenticeships), the dictionary claims **2 titles, both genuinely software**:
  zero false positives. The probe that produced these numbers ships as
  `corpus_probe_test.go`, skipped unless `CLASSIFY_CORPUS` is set, because the
  requirement now says gaps are found from production titles and a method nobody can
  re-run is not a method.

## 4. Ship

- [x] 4.1 Full local gate: `gofmt -l .` silent, `go vet ./...`, `go test ./...`,
  `go vet -tags=integration ./...`.
- [x] 4.2 PR, CI green, merge, deploy.
- [x] 4.3 Record in the project memory what the backfill owes: `cmd/backfill-derive`
  at `BACKFILL_CONCURRENCY` 2-3, then a full `make reindex`, then re-count
  `is_tech IS NULL` against today's 2,232,773 / 71,314 baseline. NOT run as part of
  this change — the host is saturated by the crawl fleet and the pass is ~15h.
