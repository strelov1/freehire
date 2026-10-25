# Screening answers

## Scope
`internal/ingest/screeninganswers` — the candidate's own answers to the six screening questions
that repeat across ATS application forms and no CV can supply: which countries they are
authorized to work in, whether they need visa sponsorship, their desired salary, their
notice period, whether they are willing to relocate, and whether they are 18 or older.
Confirmed against 443k captured `apply_forms` as the dominant repeat once standard contact
fields and demographic/EEO questions are excluded (see the `screening-answers-profile`
OpenSpec change).

## Always true
- **Not `internal/identity/userprofile`, not `internal/candidate/experience`, not `internal/candidate/resumeextract`.**
  `userprofile` is search/targeting preferences — a different lifecycle. `experience` is an
  accumulating evidence bank for CV content. `resumeextract` is CV-derived, and none of
  these six facts can be derived from a CV — they exist only because the candidate states
  them directly. Each has its own documented boundary; this package is deliberately none
  of them.
- **One row per user, six independently nullable columns — not jsonb, not free-text Q&A
  pairs.** The field set is fixed and small, so naming each fact is simpler to validate and
  read than a generic key-value store, and null unambiguously means "the candidate has not
  stated this," per field, never defaulted or guessed.
- **No provenance/confirmation state machine, unlike `internal/candidate/experience`.** Experience
  needs one because the model routinely infers achievements the candidate never confirmed.
  Here every field is a scalar fact only the candidate can state, so the write path —
  manual form or assistant tool — always carries a value the candidate themselves typed or
  spoke. Nothing here distinguishes "candidate said it" from "model inferred it" because
  the second case cannot happen through this package's API.
- **`Update` is a merge, not a replace, and there is no way to explicitly clear a field
  back to unstated.** A candidate stating only their notice period in a chat turn must not
  wipe their previously-stored salary expectation — `Merge` overlays only the fields the
  caller actually set. The missing "clear" operation is a deliberate omission: every field
  here is corrective in practice (a candidate restates a changed answer, they do not
  withdraw one), so it trades a rare, low-value operation for a simpler contract.
- **Currency is validated as well-formed, not dictionary-recognized.** Country codes go
  through `internal/dict/location.NormalizeCountry` (dict-only, unrecognized → dropped) and
  salary period through the closed `vocab.SalaryPeriodValues` enum, but `internal/dict/vocab`
  itself documents `salary_currency` as a deliberately open ISO-standard field with no
  bundled vocabulary — so currency is checked against the ISO 4217 shape (three uppercase
  letters) only. This is a narrower guarantee than the other two fields' validation, not an
  oversight.
- **`privacy_consent` is a permission, not a fact — and only a granted one leaves this
  package.** The seventh column (migration 0197) records whether the candidate lets
  auto-apply accept an employer's privacy / personal-data terms for them. `AutofillFields`
  projects it only when TRUE: a refusal answers no form (the box is a condition of applying,
  so "no" submits nothing), and omitting it is what hands the question back to the candidate.
  Never defaulted. Which fields it may answer is decided in `internal/api/atsapply/consent.go`.

## How it works
```
Sanitize (normalize country codes + currency case)
  → Validate (reject malformed input, naming the bad value)
    → Store.Update: Repository.UpdateLocked(userID, merge) — one transaction:
        Ensure row exists → SELECT ... FOR UPDATE → merge → Upsert → commit
```
`screeninganswers.go` holds the wire shape (`Answers`) and the pure `Sanitize`/`Validate`/
`Merge` functions — no database, unit-testable without one. `store.go` is the owner-scoped
`Store` over a narrow `Repository`; `repository.go` adapts `*db.Queries` + the pool to it,
mirroring `internal/identity/userprofile`'s split (single row, `PRIMARY KEY (user_id)`, `Get`
maps `pgx.ErrNoRows` to `ErrNotFound`) except for the write path.

**`Update` is a locked read-merge-write, not two separate calls.** Two consumers write
through the same `Store.Update`: the manual-edit HTTP handler and the assistant's
`screening_answers_set` tool — both documented as writing through this one path, which
means two of them can race the same `userID` for real. `Store.Update` no longer calls
`Repository.Get` then `Repository.Upsert` as two independent statements (a caller could
read the same "existing" row between them and lose the other write); it calls the single
`Repository.UpdateLocked(ctx, userID, merge)`, whose `QueriesRepository` implementation
opens one transaction, first calls `EnsureScreeningAnswersRow` (`INSERT ... ON CONFLICT
(user_id) DO NOTHING`) so the row always exists before the lock is taken — `FOR UPDATE`
locks nothing on an absent row, so without this step two concurrent *first* updates for
the same brand-new user would each read `Answers{}` and race the unguarded upsert instead
of serializing. With the row guaranteed present,
`GetScreeningAnswersForUpdate` (`SELECT ... FOR UPDATE`) takes its lock, `merge` runs
exactly once with what it locked, and `UpsertScreeningAnswers` commits the merged result
before releasing the lock. A second concurrent `Update` for the same user — whether or not
either has written before — blocks on `EnsureScreeningAnswersRow`/`FOR UPDATE` until the
first commits, so it merges onto the first write's result instead of racing it on the same
stale (or absent) read — no new schema or version column, since Postgres's own row-level
locking is the serialization point.
