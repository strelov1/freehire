## Context

`(*API).autofillProfile` (`internal/handler/autofill_profile.go:74`) assembles the
extension's contact block from two hand-written statements — `SELECT email FROM users` and
`SELECT data FROM cvs WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 1` — followed by a
hand `json.Unmarshal` into `cv.Document`. They are the only raw SQL left in
`internal/handler`.

Two facts from production (`hire` on host-2, measured 2026-08-02) set the shape of this
change:

| | all users | API-key holders |
|---|---|---|
| total | 408 | 17 |
| current structured résumé | 174 | 8 |
| base CV | 10 | 1 |
| CVs at all (any kind) | 10 | — |
| tailored CVs but no base | **0** | 0 |

So the source the code reads is present for 10 users while the source it ignores is present
for 174, and every structured résumé on production is current (all 174 stamps match
`resume_uploaded_at`). The tailored-only case the original finding demanded a decision
about does not exist in the data.

Three readers already exist and are the whole implementation: `cv.Store.BaseCV`
(`internal/cv/store.go:255`), `resume.Store.Structured` (`internal/resume/resume.go:239`,
which applies the stamp-freshness rule and treats a corrupt blob as absent) and
`GetUserByID` (`internal/db/queries/users.sql:21`).

## Goals / Non-Goals

**Goals:**

- Serve the contact block from the base CV, then the structured résumé, then the account
  email alone — so the 164 users whose data is already parsed stop getting an empty form.
- Read through `cv.Store`, `resume.Store` and sqlc; leave `internal/handler` with no raw SQL.
- Keep the nine wire fields byte-identical, so no extension release is coupled to this.

**Non-Goals:**

- **Attaching the CV PDF to a form.** The agent's primitive vocabulary is `read_form`,
  `fill_simple` and the four combobox steps; there is no file primitive in
  `internal/browsertools`. Uploading the tailored PDF for the vacancy being applied to is a
  real and larger feature, and it is where the tailored-vs-base question genuinely bites.
  Out of scope here and worth its own proposal.
- **Making autofill vacancy-aware.** Neither entry point accepts a job today. With only the
  contact block on the wire, a `?job=` parameter would change exactly one field
  (`location`), and change it in a debatable direction.
- **Reading `userprofile`.** Its `LocationPreferences` answers "where would you move",
  not "where do you live". It is not a contact source.
- **Backfilling anything.** This change writes nothing.

## Decisions

### D1. The first source that states a contact answers for the whole block

Rejected: merging field-by-field across sources. `cv.Seed` (`internal/cv/seed.go:18-25`)
copies the structured résumé's five contact fields into the CV header one-for-one, so the
two sources hold the same fields with the CV holding the corrected values. A merge would
therefore restore precisely what the candidate deleted — clear a stale phone from the CV
header and the résumé would put it back. Whole-block selection makes "the candidate's copy
wins" true rather than approximately true.

"States a contact" rather than "the row exists" is deliberate: a CV created empty (not
seeded) would otherwise silence a résumé that has the values.

### D2. The base CV outranks the structured résumé

The CV header is the candidate-authored copy and the only one the tailoring agent cannot
touch — `cvedit.DefaultPolicy()` (`internal/cvedit/policy.go:33-45`) denies `ActorAgent`
exactly `header.full_name`, `header.email`, `header.phone` and `header.links`, and denies
the candidate nothing. The structured résumé is upstream of it: contact fields there come
from deterministic PII detection over the uploaded file, per the `resume-structured-profile`
requirement, and flow into the CV through `Seed`. Corrected beats derived.

### D3. Tailored CVs are excluded, and the exclusion comes from `BaseCV`, not a new query

`GetBaseCVByUser` (`internal/db/queries/cvs.sql:70-78`) already filters `NOT is_tailored`
and documents why it excludes on that column rather than on `job_id IS NULL` (prune nulls
the vacancy link). Using `BaseCV` inherits that reasoning instead of restating it.

`BaseCV` returning `ok=false` for a user whose only CVs are tailored is not a case that
needs a policy: the next tier answers, and production has zero such users.

### D4. A feature handler owns the two routes

`internal/handler/AGENTS.md` gives each feature a handler struct with its own
dependencies and says `API` carries "only the cross-cutting dependencies", yet the three
autofill routes hang off `*API` and drove the reach for `a.pool` in the first place. A
small `autofillHandlers{cvStore, resumeStore, queries, browserTools, llm}` with a
`register` matches the package convention and takes the feature deps off `API`.

Two consequences worth stating:

- `llmBinding` is autofill's alone (`a.llm` is read only at `autofill_agent.go:32`) and is
  currently assembled across two Register statements (`handler.go:292` and `:355`).
  Constructing the handler after both are known makes it one constructor argument and
  removes the two-phase assignment.
- `browserTools` **stays** on `API` and is passed in: `/tools/ws` (`browsertools.go:49`)
  and the assistant (`handler.go:343`) share the same hub. It is genuinely cross-cutting.

### D5. `cv.NewStore` is hoisted into `Register`

Tier 1 needs `*cv.Store`, which is built inside `newCVHandlers` (`internal/handler/cv.go:80`)
and dug back out at `handler.go:394`. Reaching into `cvH.cvStore` for a second consumer
would add another such reach-in; hoisting it to a `Register` local is what
`internal/handler/AGENTS.md:26` already says should be true of the CV store. `resume.Store`
needs nothing — it is already a Register local (`handler.go:270`).

This is the minimum needed to get the dependency, not a wider rewiring: the CV renderer,
`assistant.Store` and `moderation.Service` are left where they are.

## Risks / Trade-offs

- **A user's CV header is worse than their résumé's** (they seeded a CV, cleared the phone
  by accident, and the résumé still has it) → they now get the empty phone. This is the
  intended direction: the CV is the copy they control, and honouring a deletion is the
  point of D1. The failure mode of the opposite choice — resurrecting a removed number — is
  worse and silent.
- **The structured résumé's contacts are only as good as the PII detector** → the tier is
  reached only when the candidate has no CV, so a wrong value is never preferred over one
  they authored; and today the alternative for those 164 users is an empty form, not a
  correct one.
- **A stale structure is served** → cannot happen through `resume.Store.Structured`: it
  compares `resume_structured_uploaded_at` against `resume_uploaded_at` and reports absent
  on a mismatch. Verified on production: 174 of 174 stamps match.
- **The extension breaks on a changed payload** → the nine fields and their JSON names are
  unchanged; only the values change, and only from empty to populated for the users this
  targets.

## Migration Plan

1. No migration. No schema change, no backfill, nothing written.
2. Deploy normally (blue/green). The change takes effect on the next request.
3. Verify against production by comparing the two colours: read
   `GET /api/v1/me/autofill-profile` for a key-holding account with a structured résumé and
   no CV — the inactive colour is the "before" reference and must return email-only, the
   new one must return the parsed name and phone.
4. **Rollback:** revert the deploy. Nothing persists, so there is nothing to undo.

## Open Questions

None blocking. One recorded for later: the PDF-attachment feature named in Non-Goals is the
question this change deliberately does not answer, and it is the one where choosing between
the base and the vacancy's tailored copy actually matters.
