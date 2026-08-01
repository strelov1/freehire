## 1. Move the mapping to the hash

- [x] 1.1 Confirm the two copies are still byte-identical before deleting either.
- [x] 1.2 Add `jobhash.OfRow(j db.Job, description string) string`, with the doc comment saying
      WHY it lives beside `Of` — the two are one decision split in half.
- [x] 1.3 Delete both `hashParams` copies; point the two call sites and the four test call sites
      at `OfRow`.

## 2. Replace the reviewer with a test

- [x] 2.1 `TestOfRow_CarriesEveryFieldTheHashReads`: mutate one hashed field at a time on a
      fully-populated row and assert the fingerprint moves. A dropped field cannot move it.
- [x] 2.2 Prove the guard fires rather than assuming: remove `Seniority` from the mapping and
      confirm the test fails naming that field.

## 3. Verify and close

- [x] 3.1 `go test ./...` AND `go test -tags=integration ./...` — CI runs both, and the second is
      the one that caught the last refactor's bug.
- [x] 3.2 Confirm no `hashParams` remains anywhere.
- [x] 3.3 Record what was deliberately left: `Of` under-covers the search document by three
      fields, and closing that gap invalidates every stored hash.
- [x] 3.4 Mark S14 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
