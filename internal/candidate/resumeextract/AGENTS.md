# Structured résumé extraction conventions

## Scope
Best-effort, read-only LLM parse of the stored user CV into a typed `Structured` shape. Extracted in the background on every CV upload.

**It no longer owns work experience.** Since the experience-bank change the parse also feeds `internal/candidate/experience`, which is the durable home of what the candidate has done; this package keeps the sections that have no accumulation problem — contacts, summary, education, languages, links, the total-years estimate. The staleness rule below governs those and only those.

## Always true
- **Derived in the background on every upload** (both `PutResume` and `ExtractResumeProfile`), folded into `deriveResumeArtifacts` beside `embedResume` so the two paths can't drift.
- **Staleness governs the sections this package still owns, and NOT the work history.** Experience is imported into the bank, which is additive and unstamped: a pending extraction, a superseded structure or a deleted résumé must not hide banked experience. Before the bank, a stale structure took the whole fit analysis with it.
- **Staleness is keyed on upload time ALONE, not the model stamp.** A superseded structure reads as absent (self-healing on the next extract), the same stamp-and-compare discipline as the matchanalysis cache.
- **`resume.Store.Structured` serves ONLY while the derive stamp equals the current `resume_uploaded_at`.**
- **Write is monotonic:** `SetUserResumeStructured ... WHERE resume_uploaded_at = $stamp` — a slow extraction for an already-replaced CV is dropped instead of clobbering the fresh one (a lost-update that would otherwise hide the structure forever).
- **`Sanitize` is the persist guard AND the prompt-injection guard:** bounds every string, caps arrays, coerces years. Same "never persist an out-of-vocabulary value" invariant as enrichment. It stays on the schema path — a schema bounds neither string length nor array size.
- **The request carries a schema derived from `Structured`** (`schema.go`), minus the contact fields: those come from PII detection over text the model never sees, and a strict schema requires every property, so leaving them in would be an instruction to invent a name, email, phone, residence or links for a CV that reaches the model with those redacted. Candidate `location` is restored from the first `ADDRESS` span the same way — it must not be left for the model after the residence line is masked.
- **`total_years` is asked for as TEXT, though the contract field is an `int`.** Given an integer field a model turns "5.9 years" into 6 by rounding; `truncInt` turns it into 5. The difference is experience the candidate does not have, so the arithmetic stays on this side.
- **An unconfigured/failing LLM leaves upload, embedding, and the deterministic extractors untouched.**
- **Persisting the structure also derives the candidate's GEOGRAPHY** from its `location`
  line, in the same statement and under the same stamp (`resume.Store.SetStructured` →
  `resume.DeriveGeography` → `location.ParseResidence`). Riding in one statement means the
  monotonic guard covers both, so a stored geography can never describe a different CV than
  the structure beside it. Unlike the structure, the geography HAS a reconciler
  (`cmd/backfill-resume-geo`, database-only), so a dictionary change reaches existing users.
- **Deletion clears the columns** (`ClearUserResume`), geography included.
- **No new env** — reuses `LLM_*`.
- **The fit analysis reads a COMPOSITION, not this shape alone:** `experience.Store.Professional` takes the work history from the bank and everything else from here, and that is the only candidate text `matchanalysis` sends. (It named `ProfessionalFrom` until 2026-08-01 — a second implementation of the same composition that no production path called.) An empty bank means no analysis; a stale structure now costs education and languages rather than the whole thing. There is still deliberately no raw-CV fallback, and there is deliberately no fallback to this package's own copy of the experience either — a silent one would hide a failed backfill for months.

## How it works

`internal/candidate/resumeextract` is a self-contained prompt unit like `internal/candidate/matchanalysis`/`internal/ai/enrich`, NOT an agent. It turns the uploaded CV into a typed `Structured` (contacts, summary, work experience with structured `internal/candidate/perioddate.PeriodDate` dates, education, languages, links, total years) via the shared `internal/platform/llm` client.

**File split:** `structured.go` holds the wire shape + `Sanitize` + the `Professional` projection. `resumeextract.go` holds the server-only `Extractor` — split so `cmd/gen-contracts` emits only `structured.go`, mirroring `matchanalysis.go` vs `analyzer.go`.

**`Professional` is the contact-free projection** of `Structured` — everything except `full_name`/`email`/`phone`/`links`. Its field set is a **whitelist**, so a field added to `Structured` is withheld until it is added to the projection too; a blacklist over the known contact keys would disclose each new field by default. **Every** de-identified consumer goes through it, and takes it as the typed value rather than a JSON string it re-filters: the fit chain's candidate context (`matchanalysis.Input.StructuredResume`), the ATS review (`atscheck.Analyze`), the profile read's `cv` block, and the assistant's `get_profile`. The ATS review used to be the exception — it received the full `Structured` marshalled and deleted four known keys, the blacklist this comment argues against; it leaked nothing only because the complement of `Professional` happened to be exactly those four. `professional_test.go`'s `withheld` map is the tripwire: adding a field to `Structured` fails it until somebody decides which side the field belongs on.

**Persistence:** stored read-only per user on `users` (`resume_structured` jsonb + `resume_structured_model` + `resume_structured_uploaded_at`, migration `0011`), stamped with the résumé upload time it was derived from (captured up front, not `now()`). The `resume_structured_model` column is kept only as provenance for a future backfill.

**Serving — three surfaces now, and the widening trust boundary is the point:** the full structure, contacts included, is served only by `GET /api/v1/me/resume`, which is **cookie-only** (rendered read-only in the profile's readiness tab, `ResumeStructuredView.svelte`). The `Professional` projection is served by `GET /api/v1/me/profile` as its `cv` block, which **accepts an API key**, so a credential in an agent's environment reads the substance and never the identity. Both are null when absent/stale/unconfigured. The projection also feeds the fit chain via `matchanalysis.Input.StructuredResume` — the sole (de-identified) candidate context — and the assistant's `get_profile` tool, where omitting contacts matters most: a tool result is persisted in the session transcript and replayed into the model's context on every later turn.

The third surface is `visibility.go`'s `Anonymous()`/`Public()` (the talent-network-profile-visibility change), served by `GET /api/v1/talent-network/:publicID` — **no auth at all**, the open internet. Both build on `Professional()` and each adds exactly one further redaction of its own (`Anonymous()` also masks a current employer and strips every `Project.Link`; `Public()` also strips `Project.Link`) — a project's link is de-anonymizing the same way the four contact fields are, and it lived on `Structured`/`Professional` unredacted until a whole-branch review caught it reaching this route. The consequence: a field added to `Structured` and then to `Professional` (a defensible call for the first two, authenticated/keyed surfaces) now ships to this third one **by default**, with nobody forced to re-decide whether an anonymous or public viewer should see it too. `professional_test.go`'s `TestProfessional_IsAWhitelist` only guards the second surface's boundary. `visibility_test.go`'s `TestAnonymous_IsAWhitelist`/`TestPublic_IsAWhitelist` are the tripwire for this one — they enumerate the exact JSON key set `Anonymous()`/`Public()` actually emit, and fail the moment that set changes without the diff being reviewed for public exposure.

**Wire shape:** generated to TS via `cmd/gen-contracts`.

**Staleness rationale for model stamp:** unlike the CV embedding (re-checked against current embedder), the structure has no reconciler that re-derives it — only a re-upload does. So gating reads on the model would hide the parsed profile forever after an `LLM_MODEL` upgrade. Serving a best-effort display-only structure from an older model is the better degradation.

## Limitations
- `resume_structured_model` is provenance for a future backfill (the noted seam) — no reconciler re-derives the structure.
- Migration `0011` must be applied to prod manually before deploy.
