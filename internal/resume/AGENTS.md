# Résumé identity

## Scope
`internal/resume` — one stored CV per user, the structured extract stamped to that
upload, and `Owned` — the candidate-owned override block that outlives the file and now
covers identity *and* the flat/scalar part of the semantic body (headline, summary,
languages, certifications, education), not identity alone. See `owned.go`'s own doc
comment for the field-by-field boundary (why Skills/Experience stay outside it).

## Three layers

Owned-overridable fields (name, email, phone, location, links, headline, summary,
languages, certifications, education) have exactly three sources. Read-time compose
picks one block, not a field-by-field mix across sources — but Owned itself DOES win
field by field against whichever source it's layered over, since a candidate editing
their email must not blank their independently-edited summary. Persist-time
`FillEmptyOwnedFromStructured` may fill blanks on a new extract.

| Layer | What it is | Survives delete | Everything else |
|---|---|---|---|
| **Owned overrides** | `users.candidate_contacts` (column keeps its original name), written by the candidate (or fill-empty from a current extract) | yes | never |
| **Current structure** | `resume_structured` whose stamp equals `resume_uploaded_at` | no | yes — skills, projects, years (the fields Owned does NOT cover) |
| **Provisional contacts** | identity-only slice of a superseded blob while extract is pending or failed | no | never |

Precedence on Profile, CV seed, and header heal: **owned block** (per-field, over whichever of the other two is in play) → else current extract → else provisional contacts.

A reader that needs “is the file-owned parse current?” uses stamp-gated `Structured` (`ok` is false while pending). A reader that needs identity-or-owned-body uses the precedence above. No fourth mix.

## Who may use which layer

| Reader | Contacts | Semantic body | Experience / projects |
|---|---|---|---|
| `Structured` | current stamp only; else absent | current stamp only | current stamp only |
| `ProfileReadForUser` | current, else provisional slice | current only | ignored here; `GET /me/resume` overlays the bank |
| `ProvisionalContacts` | superseded blob, identity only | never | never |
| `CandidateContacts` | owned column | never | never |
| `StructureForSeed` | owned overlay, else current or provisional | current only (stale stripped) | current only; bank overlay is `bankedSeeder` |
| `bankedSeeder.Structured` | via `StructureForSeed` | via `StructureForSeed` | bank jobs/projects when present; else structure |
| `applySeedContent` / `mergeSeedHeader` | seed-first (empty seed keeps the CV field) | whole-document replace | from seed |
| `healRecordHeader` / `fillEmptyHeaderFields` | keep-first fill from identity | untouched | untouched |
| `reseedBaseIfStaleVsUpload` | full reseed if current; heal-only if pending | same | same |

`GET /me/resume` composes Profile: owned contacts field + `ProfileReadForUser` + bank `SeedHistory`. That composition must match `StructureForSeed` + `bankedSeeder` for identity and for “stale ⇒ no superseded body.”

## Always true
- **Stamp gate is not identity.** A pending extract is not a current structure. Provisional contacts may still fill a name.
- **Owned contacts win as a block** on compose. Mixing a typed email with a stale extract name is the bug this table exists to prevent.
- **Delete clears the file and the extract, not owned contacts.**
- **Two header merges stay separate.** Reset means “match the résumé” (seed-first). Heal means “do not keep showing a blank name we already know” (keep-first). Sharing one function picks the wrong winner.
- **Owner GET of a CV may persist a heal.** It is a blank-header repair: idempotent, body untouched, list/PDF do not write. See `healRecordHeader`.
