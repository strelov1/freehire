## 1. Confirm it is really dead

- [x] 1.1 Repo-wide grep for every reference, including docs — declaration plus seven test call
      sites, no `cmd/` or `internal/` non-test caller.

## 2. Delete the function, keep what it knew

- [x] 2.1 Move the rationale onto `Store.Professional`: the bank/structure division, the
      deliberate absence of a fallback, and the contact whitelist. Deleting the function must not
      delete the reasoning — that is the part a future reader needs.
- [x] 2.2 Delete `ProfessionalFrom`.

## 3. Re-point the tests at the live path

- [x] 3.1 To `Store.Professional`, not `experienceFromBank`: the seven rules are worth asserting
      through the store reads the fit chain actually makes.
- [x] 3.2 A `seedBank` helper where atoms name their employment by INDEX, since the store mints
      the ids — and a `professionalOf` helper so each test reads as one assertion.

## 4. Fix the doc that pointed at the dead one

- [x] 4.1 `internal/resumeextract/AGENTS.md` names `Store.Professional`, and says what the old
      name was, so this does not read as a rename.

## 5. Verify and close

- [x] 5.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 5.2 Mark S19 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
