## 1. Find out which side is actually right

- [x] 1.1 Run both implementations over the same URLs rather than reading them, and record every
      divergence with which side is correct.
- [x] 1.2 Check whether any divergence is a deliberate, tested decision before treating it as a
      gap — none of the six was covered by an `atsboard` test.

## 2. Fix the shared table first

- [x] 2.1 Locale match accepts a lowercase country half; a Workday path leading with
      `job`/`details` carries no site.
- [x] 2.2 `noBoardFirstSegments` declines Workable's `/j/<id>`. Not `reservedSegments`: skipping
      `j` reaches the job id and takes it as the board, which is worse than declining.
- [x] 2.3 `modePathNumeric` for PageUp's numeric institution id.
- [x] 2.4 `subdomainLabel` declines a multi-label remainder; `subdomainchain` remains for the
      platforms that genuinely nest a tenant under an instance.
- [x] 2.5 A test for each — this table had none for workable, pageup or cornerstone at all.

## 3. Then delegate

- [x] 3.1 `FromURL` calls `atsboard.Recognize` first; delete the eleven overlapping cases and the
      helpers only they used.
- [x] 3.2 Keep the five shapes `atsboard` excludes, with the reason (the accept-set pays) written
      in both packages.
- [x] 3.3 The existing 30-URL `TestFromURL` corpus is the pin: it must pass unchanged, and it is
      what caught all three remaining divergences.
- [x] 3.4 A test that the two sets stay disjoint in both directions.

## 4. Verify and close

- [x] 4.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 4.2 Note that the finding's `boardresolve` evidence has gone stale — the inline
      `matched && b != "embed"` is gone; that path already goes through `Recognize`.
- [x] 4.3 Mark S17 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
