## 1. Implementation

- [x] 1.1 Add the `pending`/`pendingHasProse` buffer state and `flushPending`/`resetPending` helpers to `Derive`'s walk in `internal/job/reqextract/reqextract.go`.
- [x] 1.2 Buffer a too-long (real-prose) text block instead of closing the section immediately; buffer a short unrecognized inline line without marking `pendingHasProse`.
- [x] 1.3 On a recognized heading transition and on a table, flush the buffer under the closing priority via `flushPending` (commits only when 2+ items).
- [x] 1.4 On a list found while `pendingHasProse` is set, discard the buffer and close the section instead of reading the list; otherwise clear any buffered lead-in before reading the list as before.
- [x] 1.5 Flush the buffer once more after the walk completes, for a section with no trailing heading/table.

## 2. Tests

- [x] 2.1 Update the pre-existing `tbank.ru`-shape negative test to assert the two now-derived requirements instead of `nil`.
- [x] 2.2 Add boundary-case tests: exactly two paragraph items commit; a single unmatched paragraph yields nothing; a mix of short and long paragraphs all commit; a paragraph-shaped preferred section commits with `preferred` priority; a paragraph-shaped section under a non-vocabulary heading yields nothing; a paragraph run with no trailing heading commits at document end; a paragraph run closed by a table still commits.
- [x] 2.3 Run `go test ./internal/job/reqextract/... -v -run TestDerive` and confirm every pre-existing case plus the new ones pass with no regressions.
- [x] 2.4 Verify against real data: run `Derive` over 25 real `tbank.ru` posting descriptions (sampled live, `Требования` heading, paragraph-per-item shape) and confirm each yields the expected requirement items.

## 3. Documentation

- [x] 3.1 Update `internal/job/reqextract/AGENTS.md`'s "How it works" section to describe the buffering mechanism.
- [x] 3.2 Remove the "Limitations" bullet documenting the now-fixed `<p>`-per-item gap.
- [x] 3.3 Flag the "Measuring coverage" section's 28.0%/29.3% figures as predating this fix.

## 4. Verification

- [x] 4.1 `gofmt -l .` clean (repo-wide, excluding node_modules).
- [x] 4.2 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` all clean.
- [x] 4.3 `go test ./...` (whole repo) green, 0 FAIL.
