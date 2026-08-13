# Screening answers

## Scope
`internal/screeninganswers` — the candidate's own answers to the six screening questions
that repeat across ATS application forms and no CV can supply: which countries they are
authorized to work in, whether they need visa sponsorship, their desired salary, their
notice period, whether they are willing to relocate, and whether they are 18 or older.
Confirmed against 443k captured `apply_forms` as the dominant repeat once standard contact
fields and demographic/EEO questions are excluded (see the `screening-answers-profile`
OpenSpec change).

## Always true
- **Not `internal/userprofile`, not `internal/experience`, not `internal/resumeextract`.**
  `userprofile` is search/targeting preferences — a different lifecycle. `experience` is an
  accumulating evidence bank for CV content. `resumeextract` is CV-derived, and none of
  these six facts can be derived from a CV — they exist only because the candidate states
  them directly. Each has its own documented boundary; this package is deliberately none
  of them.
- **One row per user, six independently nullable columns — not jsonb, not free-text Q&A
  pairs.** The field set is fixed and small, so naming each fact is simpler to validate and
  read than a generic key-value store, and null unambiguously means "the candidate has not
  stated this," per field, never defaulted or guessed.
- **No provenance/confirmation state machine, unlike `internal/experience`.** Experience
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
  through `internal/location.NormalizeCountry` (dict-only, unrecognized → dropped) and
  salary period through the closed `vocab.SalaryPeriodValues` enum, but `internal/vocab`
  itself documents `salary_currency` as a deliberately open ISO-standard field with no
  bundled vocabulary — so currency is checked against the ISO 4217 shape (three uppercase
  letters) only. This is a narrower guarantee than the other two fields' validation, not an
  oversight.

## How it works
```
Sanitize (normalize country codes + currency case)
  → Validate (reject malformed input, naming the bad value)
    → Store.Update: Get existing (ErrNotFound reads as fully-unstated) → Merge → Upsert
```
`screeninganswers.go` holds the wire shape (`Answers`) and the pure `Sanitize`/`Validate`/
`Merge` functions — no database, unit-testable without one. `store.go` is the owner-scoped
`Store` over a narrow `Repository`; `repository.go` adapts `*db.Queries` to it, mirroring
`internal/userprofile`'s split exactly (single row, `PRIMARY KEY (user_id)`, `Get` maps
`pgx.ErrNoRows` to `ErrNotFound`, `Upsert` is conflict-free by construction).

Two consumers write through the same `Store.Update`: the manual-edit HTTP handler and the
assistant's `screening_answers_set` tool. Both accept a partial `Answers` and get the same
merge-and-validate behavior — there is exactly one write path, not two.
