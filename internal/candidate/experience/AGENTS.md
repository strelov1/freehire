# The experience bank

## Scope
`internal/candidate/experience` — the durable, per-user record of what a candidate has actually
done. An `Employment` is a place where something happened (a job or a project); an `Atom`
is one piece of evidence at the grain of a CV bullet.

## Always true
- **The bank is a store, not a cache.** Every other record of experience is derived and
  replaced: `resume_structured` is regenerated on each upload, a tailored CV is thrown away
  with its vacancy. The bank accumulates, and **only its owner removes anything** — import
  never deletes, and there is no reconciler that prunes.
- **The label is DERIVED from the Author, never taken from the caller's value.** `Store.AddAtom`
  and `UpdateAtom` take an `Author` — who is asserting the claim, decided by the entry point
  that received the request — and overwrite `Atom.Provenance` from it. A body-supplied
  provenance is inert. Same rule and same reason as `cvedit.Actor`: a caller naming itself is
  not evidence of who it is.

  | Author | Label | Who passes it |
  |---|---|---|
  | `AuthorCandidate` | `manual` | the person, from their own session (cookie POST/PUT) |
  | `AuthorQuoted` | `stated_in_chat` | the assistant, after checking the words against the session transcript — **the only authorship that can raise standing** |
  | `AuthorAgent` | `agent_inferred` | a model's own reading, asserted now |
  | `AuthorRewrite` | keeps the stored label | a keyed edit: rewriting words is not re-asserting them. An absent or corrupt label falls to `agent_inferred` |

  Anything unrecognised — a new entry point that forgets to name itself — lands on
  `agent_inferred`, which the CV evidence gate refuses. Failing closed is the point.

  This closes the laundering route the cookie-only gate used to guard by routing alone: bank an
  inference as `agent_inferred`, edit it, read back `manual`, cite it in a CV.

- **Provenance decides publication, and the check is in the service.** `cv_import`,
  `stated_in_chat` and `manual` are the candidate speaking and may reach a CV;
  `agent_inferred` is the model speaking and may not, until the candidate confirms it and
  it is re-stamped. An unknown provenance is never publishable — it fails closed.
- **Import is additive.** Employments match case-insensitively on `(company, role)` and only
  their BLANK fields are filled; atoms match on the normalized claim and duplicates are
  skipped. A user who uploads a trimmed one-page CV must not lose the bank they built.
- **Listing is reverse-chronological in the domain, not via `ORDER BY period_start`.** Dates
  stay free-form display labels (`"October 2018"`, `"2024"`). Lexicographic SQL order puts
  month names above years; `ListEmployments` re-sorts with a best-effort parsed key
  (`internal/candidate/experience/period_sort.go`) so WorkHistory / CV seed / Professional agree.
  Placeless publishable atoms are appended **after** dated roles (empty company/title,
  highlights only) — they are not a titled job. A role-only row with empty company is a
  normal employment from import/edit, not that placeless path.
- **`is_current` is never written by import.** A CV that still reads "Present" for a role
  the user has ended would otherwise resurrect it.
- **Ownership is a `WHERE user_id = $1`.** An entry the caller does not own is reported as
  missing, never as forbidden.
- **`Sanitize` is the persistence guard AND the prompt-injection guard**, and it also
  coerces nil slices to empty ones — the array columns are `NOT NULL`, and pgx sends a nil
  slice as SQL `NULL`, which a column DEFAULT does not cover.
- **Skills are canonical slugs from `internal/dict/skilltag`** — the same dictionary that
  produces the `jobs.skills` facet, so matching evidence to a vacancy requirement is a set
  intersection rather than a text-matching problem.

## How it works

```
upload ──► resumeextract.Extract ──► EntriesFromResume ──► Import
                                                             │ employments: Find → FillBlanks | Create
                                                             └ atoms: InsertAtomIfNew (ON CONFLICT DO NOTHING)

requirement ──► Retrieve ──► scored, ranked, top-N ──► the agent cites an id in cv_edit
bank + structure ──► Professional ──► matchanalysis · /me/profile · cv.Seed
```

**Project wire.** Storage keeps one place-label column (`Company`) for both kinds.
JSON is kind-aware (`employment_json.go`): jobs emit `company`, projects emit
`name` and `link` and omit `company`. Inbound project JSON accepts `name`, with
legacy `company` as a fallback into the same stored field. Clients must not
assume projects expose `company`.

**File split.** `experience.go` holds the wire shape, `Sanitize`, `Validate` and
`ClaimKey`, so `cmd/gen-contracts` can emit the TS types from that file alone.
`store.go` is the owner-scoped domain surface over a narrow `Repository`; `repository.go`
adapts `*db.Queries` to it. `import.go` is the reconciliation. `retrieve.go` is the
scoring. `import_resume.go` and `professional.go` are the two adapters that name a
concrete source shape — `Store` itself knows nothing about résumés.

**Two entries into the skill dictionary, and they are not interchangeable.**
`skilltag.Parse` mines prose and applies the corroboration rule (an ambiguous word like
"react" tags only beside a strong tech token). `skilltag.Canonicalize` resolves tokens a
caller asserted, skips corroboration, and accepts an already-canonical slug. Import uses
`Parse` on a bullet's text and `Canonicalize` on a role's stack; the tools use
`Canonicalize` on what the model supplies.

**Retrieval performs no model call.** Scoring is a linear pass: canonical skill
intersection (dominant), the role's stack (weaker — a bullet silent about MongoDB still
answers a MongoDB requirement when the role ran on it), non-stopword overlap (the floor,
and the reason retrieval cannot be a skill prefilter — "led a team of five" names no slug).
Zero-scoring atoms are omitted: a weak match reads to an agent as evidence, and evidence is
what licenses a bullet.

**`ClaimKey` normalizes case, punctuation and whitespace but NOT numbers.** It is stored
under a unique index, so a collision is a silently lost achievement — "20s to 1s" and "30s
to 1s" must not merge. Nothing stems or reorders words for the same reason.

## Boundaries

| Data | Owner | Why |
|---|---|---|
| Employments, achievement atoms | this package | accumulates across uploads and sessions |
| Education, languages, certifications, contacts, total years | `resumeextract` | stable form-shaped data with no accumulation problem |
| The CV document a user edits and renders | `internal/candidate/cv` | must be a self-contained snapshot |
| Specializations, wanted/excluded skills, location | `internal/identity/userprofile` | targeting, not evidence — a different lifecycle |

The ATS report deliberately reads the **file's** structure and not the bank: it judges the
document, and banked evidence would have it praise a CV for experience the CV never states.

## Limitations
- **Paraphrase duplicates.** Exact `ClaimKey` still only collapses identical normalised
  claims. Near-paraphrases are flagged as soft-duplicate clusters (token Jaccard on the
  claim) and the owner can merge a pair via `Store.MergeAtoms` / the Experience tab /
  `experience_merge`. Import never merges. Soft-dup is a hint, not a silent write.
- **Dangling CV citations after merge.** The keep id is stable; the loser is deleted. A
  CV that already cites the loser stays a printable snapshot; a later agent `cv_edit`
  that re-cites the deleted id fails the evidence gate. Citations are not rewritten.
- **No semantic retrieval.** A linear pass is right for tens-to-hundreds of atoms;
  `internal/ai/embed` is the open seam if that changes.
- **No STAR depth.** Atoms are CV-bullet grade. Interview-prep depth would be a layer over
  the same employments, not nullable fields added speculatively now.
