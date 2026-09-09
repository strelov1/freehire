## 1. Failing tests first

- [x] 1.1 In `internal/dict/location/workmode_test.go`, add cases to
      `TestWorkModeFromDescription` for the qualifier guard: a qualifier after the
      phrase ("the freedom to work from anywhere in the world for up to a month" →
      `""`), a qualifier before the phrase ("benefits include up to 12 days work
      from anywhere" → `""`), an unqualified phrase still resolving `remote` ("you
      can work from anywhere in the EU" — already present, keep green), a qualified
      "work from anywhere" alongside an unrelated unqualified remote phrase
      elsewhere in the same description still resolving `remote`, and an unrelated
      "up to" near `100% remote` still resolving `remote` (the guard must not widen
      past the two scoped phrases). Use the real production sentence shapes from
      proposal.md/design.md, not invented ones, matching this file's existing
      convention (see `TestRemoteContradicted`'s comment on using real sentences).
- [x] 1.2 Run `go test ./internal/dict/location/...` and confirm the new cases fail
      (red) against the current implementation, for the right reason (phrase
      guard not yet implemented) — not a typo in the test itself.

## 2. Implementation

- [x] 2.1 In `internal/dict/location/workmode.go`, add the qualifier marker and
      window for the "work from anywhere" guard (a small var/const beside
      `remoteDenialPhrases`/`denialQualifierWindow`, documented per design.md's
      Decisions — bidirectional window, "up to" as the sole marker, scoped to
      exactly the two phrases).
- [x] 2.2 Wire the guard into `WorkModeFromDescription`'s match loop: when the
      matched phrase is "work from anywhere" or "work-from-anywhere" and the
      qualifier appears within the window on either side of the match, skip this
      match (continue to the next phrase) instead of returning `"remote"`.
- [x] 2.3 Run `go test ./internal/dict/location/...` and confirm all cases pass
      (green), including the pre-existing ones (no regression).

## 3. Verification

- [x] 3.1 `gofmt -l internal/dict/location/` prints nothing.
- [x] 3.2 `go vet ./...` and `go build ./...` are clean.
- [x] 3.3 `go test ./...` is green (full unit suite — this package has no
      integration-tagged tests, but confirm no other package's tests reference
      `WorkModeFromDescription` in a way this change breaks — `jobderive_test.go`
      was identified as a caller-side consumer during triage; confirm it still
      passes unchanged).
- [x] 3.4 Re-run the design.md scenarios manually against `WorkModeFromDescription`
      (or as part of 1.1's table) to confirm every scenario in
      `specs/deterministic-facets/spec.md`'s new requirement is covered by a test
      case.

## 4. Close-out

- [ ] 4.1 Note in the PR description that reaching already-ingested jobs needs
      `cmd/backfill-derive` followed by a full `cmd/reindex` (ops follow-up, not
      part of this PR — per design.md's Migration Plan).
- [ ] 4.2 Reference freehire#2696 in the PR/commit.
