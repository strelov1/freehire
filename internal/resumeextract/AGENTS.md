# Structured résumé extraction conventions

## Scope
Best-effort, read-only LLM parse of the stored user CV into a typed `Structured` shape. Extracted in the background on every CV upload.

## Always true
- **Derived in the background on every upload** (both `PutResume` and `ExtractResumeProfile`), folded into `deriveResumeArtifacts` beside `embedResume` so the two paths can't drift.
- **Staleness is keyed on upload time ALONE, not the model stamp.** A superseded structure reads as absent (self-healing on the next extract), the same stamp-and-compare discipline as the matchanalysis cache.
- **`resume.Store.Structured` serves ONLY while the derive stamp equals the current `resume_uploaded_at`.**
- **Write is monotonic:** `SetUserResumeStructured ... WHERE resume_uploaded_at = $stamp` — a slow extraction for an already-replaced CV is dropped instead of clobbering the fresh one (a lost-update that would otherwise hide the structure forever).
- **`Sanitize` is the persist guard AND the prompt-injection guard:** bounds every string, caps arrays, coerces years. Same "never persist an out-of-vocabulary value" invariant as enrichment.
- **An unconfigured/failing LLM leaves upload, embedding, and the deterministic extractors untouched.**
- **Deletion clears the columns** (`ClearUserResume`).
- **No new env** — reuses `LLM_*`.
- **Sole candidate context of the fit analysis, never an add-on:** the structured shape (contacts stripped) is the ONLY candidate text `matchanalysis` sends to the model; a missing/failed extraction means the fit chain produces no analysis — there is deliberately no raw-CV/text-only fallback.

## How it works

`internal/resumeextract` is a self-contained prompt unit like `internal/matchanalysis`/`internal/enrich`, NOT an agent. It turns the uploaded CV into a typed `Structured` (contacts, summary, work experience with free-form dates, education, languages, links, total years) via the shared `internal/llm` client.

**File split:** `structured.go` holds the wire shape + `Sanitize` + the `Professional` projection. `resumeextract.go` holds the server-only `Extractor` — split so `cmd/gen-contracts` emits only `structured.go`, mirroring `matchanalysis.go` vs `analyzer.go`.

**`Professional` is the contact-free projection** of `Structured` — everything except `full_name`/`email`/`phone`/`links`. Its field set is a **whitelist**, so a field added to `Structured` is withheld until it is added to the projection too; a blacklist over the known contact keys would disclose each new field by default. Both de-identified consumers go through it: the fit chain's candidate context and the profile read's `cv` block.

**Persistence:** stored read-only per user on `users` (`resume_structured` jsonb + `resume_structured_model` + `resume_structured_uploaded_at`, migration `0011`), stamped with the résumé upload time it was derived from (captured up front, not `now()`). The `resume_structured_model` column is kept only as provenance for a future backfill.

**Serving — two surfaces, and the asymmetry is the point:** the full structure, contacts included, is served only by `GET /api/v1/me/resume`, which is **cookie-only** (rendered read-only in the profile's readiness tab, `ResumeStructuredView.svelte`). The `Professional` projection is served by `GET /api/v1/me/profile` as its `cv` block, which **accepts an API key**, so a credential in an agent's environment reads the substance and never the identity. Both are null when absent/stale/unconfigured. The projection also feeds the fit chain via `matchanalysis.Input.StructuredResume` — the sole (de-identified) candidate context — and the assistant's `get_profile` tool, where omitting contacts matters most: a tool result is persisted in the session transcript and replayed into the model's context on every later turn.

**Wire shape:** generated to TS via `cmd/gen-contracts`.

**Staleness rationale for model stamp:** unlike the CV embedding (re-checked against current embedder), the structure has no reconciler that re-derives it — only a re-upload does. So gating reads on the model would hide the parsed profile forever after an `LLM_MODEL` upgrade. Serving a best-effort display-only structure from an older model is the better degradation.

## Limitations
- `resume_structured_model` is provenance for a future backfill (the noted seam) — no reconciler re-derives the structure.
- Migration `0011` must be applied to prod manually before deploy.
