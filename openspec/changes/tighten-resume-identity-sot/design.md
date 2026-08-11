## Context

See proposal.md for why. The branch already implements the five product pieces (owned contacts, parse status, project employments, banked seed split, GET heal, experience tools, coverage). What is missing is a single decision table those readers must share, plus docs the review asked for.

Today the table is implicit and split:

| Reader | Contacts | Semantic body | Experience / projects |
|---|---|---|---|
| `Structured` | current stamp only; else absent | current stamp only | current stamp only |
| `ProfileReadForUser` | current, else provisional slice | current only | ignored by the reader; handler overlays bank |
| `ProvisionalContacts` | superseded blob, identity only | never | never |
| `CandidateContacts` | owned column | never | never |
| `StructureForSeed` | owned overlay, else current or provisional | current only (stale stripped) | current only; bank overlay is `bankedSeeder` |
| `bankedSeeder.Structured` | via `StructureForSeed` | via `StructureForSeed` | bank jobs/projects when present; else structure |
| `applySeedContent` / `mergeSeedHeader` | seed-first, keep if seed empty | whole-document replace | from seed |
| `healRecordHeader` / `fillEmptyHeaderFields` | keep-first fill from identity | untouched | untouched |
| `reseedBaseIfStaleVsUpload` | full reseed if current; heal-only if not | same | same |

`GET /me/resume` composes Profile: owned contacts field + `ProfileReadForUser` + bank `SeedHistory`. That composition must match `StructureForSeed` + `bankedSeeder` for identity and for “stale ⇒ no superseded body.”

## Goals / Non-Goals

**Goals:**

- One documented table in `internal/resume/AGENTS.md` that Profile, seed, heal, and reset cite.
- Readers that still diverge from the table are aligned (comments + tests first; code only where a real mismatch remains).
- Project wire (`name` vs `company`) is stated in `internal/experience/AGENTS.md` and locked by a marshal/unmarshal test (already present — confirm it matches the spec).
- GET heal stays, with an explicit justification next to the write and tests that a second GET is a no-op and list/PDF do not write.

**Non-Goals:**

- Rewriting contacts, projects, seed, experience tools, or coverage CI.
- A formal FSM library, new status enum beyond `pending` / `ok` / `failed`, or collapsing owned contacts into `resume_structured`.
- Renaming the storage column `Company` on employments.
- New migrations.
- Making GET heal a POST; the write-on-GET stays as a justified exception.

## Decisions

### 1. Document the table; do not add a fourth composer

**Choice:** Keep the existing functions. Add `internal/resume/AGENTS.md` as the architecture note the review asked for. Point `cv_seed.go`, `cv_header_heal.go`, and `GetResume` at it.

**Why not one mega-`IdentityState` type:** the call sites need different slices (stamp gate vs identity vs seedable composition). A new type would re-encode the same table and touch every fake in tests. The bug class is drift, which comments + a shared table + tests catch cheaper.

**Alternative considered:** Merge `ProfileReadForUser` and `StructureForSeed`. Rejected for this change — they already share `provisionalContacts` and owned overlay; collapsing them is a later cleanup if drift reappears.

### 2. Owned contacts win as a block, not field-by-field, on compose

**Choice:** When owned contacts are non-empty, seed/profile replace the whole contact block from owned (see `StructureForSeed` and `GetResume`). Fill-empty from a new extract still runs at persist time (`FillEmptyContactsFromStructured`) so a first parse can populate blanks.

**Why:** Field-by-field overlay on every read would mix a typed email with a stale extract name. Persist-time fill-empty is the merge; read-time is “owned block or extract block.”

### 3. Two header merges stay separate

**Choice:** `mergeSeedHeader` (seed-first) for reset / full reseed. `fillEmptyHeaderFields` (keep-first) for GET heal and pending stale-base refresh.

**Why:** Reset means “make the CV match the résumé.” Heal means “do not keep showing a blank name we already know, and do not overwrite a name the candidate typed.” Sharing one function caused `TestGetCV` to expect the wrong winner after rebase.

### 4. GET heal is an allowed write

**Choice:** Owner `GET /me/cvs/:id` may `CommitDocument` when the keep-first fill changes the header. Idempotent: second GET no-ops. List and PDF do not heal.

**Why:** Tailored copies minted before contacts existed reopen with a blank header forever unless something writes. Open/load is the moment the candidate sees the bug. A separate “repair” endpoint would never be called.

**Constraint:** Heal uses `ActorCandidate` / `OriginImport` so the revision is an import repair, not an agent edit. It must not debit credits or touch body evidence.

### 5. Project wire is kind-aware JSON, one storage field

**Choice:** Keep `Employment.Company` as the place label. `MarshalJSON` emits `name` for `kind=project` and `company` for jobs. `UnmarshalJSON` accepts `name` or legacy `company` on projects.

**Why:** Storage and matching stay `(company, role)`. The wire change is the review’s compatibility risk — document it; do not silently emit both keys (that would teach clients the wrong shape).

## Risks / Trade-offs

- **[Risk] Docs rot and readers drift again** → Mitigation: AGENTS.md table is the cited contract; add one table-driven test that names each reader’s outputs for the same fixture (current / pending / owned / deleted).
- **[Risk] GET heal surprises clients that assume GET is read-only** → Mitigation: spec + comment on `healRecordHeader`; persist only when the header actually changes; no list/PDF writes.
- **[Risk] Clients still send `company` on projects** → Mitigation: inbound fallback stays; outbound omits `company`; document in experience AGENTS and any generated TS comments if present.
- **[Risk] Scope creeps into a rewrite** → Mitigation: tasks are docs, comment alignment, and gap tests. Code changes only when a reader contradicts the table.

## Migration Plan

No schema change. Deploy is docs + tests on top of the existing PR migrations (`users.candidate_contacts`, extract status, employment `kind` / `link`). Rollback is revert the doc/test commit; behavior stays as the PR already shipped.

## Open Questions

None that affect specs or task breakdown. Whether to later collapse `ProfileReadForUser` and `StructureForSeed` is deferred.
