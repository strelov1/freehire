## 1. Check each claim against the world, not the finding

- [x] 1.1 #32's flock claim: grep Go, then read the actual systemd units on production. `flock`
      appears in neither — but systemd's oneshot serialization does the job for timer-started
      runs, which is a better correction than deleting the sentence.
- [x] 1.2 #32's "the project has none": enumerate the real advisory-lock users and their keys.
- [x] 1.3 #16's erasure: measure it on production rather than reason about it — 7 manual rows,
      all 7 still carrying regions.

## 2. Put each fact where someone will read it

- [x] 2.1 The lock-key list in `internal/migrate`, with both other sites pointing at it.
- [x] 2.2 `CLAUDE.md`: what serializes cron workers, and that a manual run is unprotected.
- [x] 2.3 `cmd/backfill-derive`: regions/cities are re-derived like the other structured facets,
      it is not this command's decision alone, the measured radius, and where the fix goes if the
      intent changes — both doors, never one.

## 3. Verify and close

- [x] 3.1 `go build`, `go vet`, `go test ./...` and `-tags=integration` — comment-only, but the
      claims are about code that must still compile and pass.
- [x] 3.2 Mark #16 and #32 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
