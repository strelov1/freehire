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
- **Additive to fit analysis, never a replacement:** the structured shape is fed as Stage-1 context to `matchanalysis`; missing/failed extraction degrades to text-only analysis.

## How it works

`internal/resumeextract` is a self-contained prompt unit like `internal/matchanalysis`/`internal/enrich`, NOT an agent. It turns the uploaded CV into a typed `Structured` (contacts, summary, work experience with free-form dates, education, languages, links, total years) via the shared `internal/llm` client.

**File split:** `structured.go` holds the wire shape + `Sanitize`. `resumeextract.go` holds the server-only `Extractor` — split so `cmd/gen-contracts` emits only `structured.go`, mirroring `matchanalysis.go` vs `analyzer.go`.

**Persistence:** stored read-only per user on `users` (`resume_structured` jsonb + `resume_structured_model` + `resume_structured_uploaded_at`, migration `0011`), stamped with the résumé upload time it was derived from (captured up front, not `now()`). The `resume_structured_model` column is kept only as provenance for a future backfill.

**Serving:** exposed on `GET /api/v1/me/resume` (new `structured` field, null when absent/stale/unconfigured). Rendered read-only in the profile's readiness tab (`ResumeStructuredView.svelte`). Fed into the fit chain as `matchanalysis.Input.StructuredResume` — pre-normalized Stage-1 context beside the raw CV text.

**Wire shape:** generated to TS via `cmd/gen-contracts`.

**Staleness rationale for model stamp:** unlike the CV embedding (re-checked against current embedder), the structure has no reconciler that re-derives it — only a re-upload does. So gating reads on the model would hide the parsed profile forever after an `LLM_MODEL` upgrade. Serving a best-effort display-only structure from an older model is the better degradation.

## Limitations
- `resume_structured_model` is provenance for a future backfill (the noted seam) — no reconciler re-derives the structure.
- Migration `0011` must be applied to prod manually before deploy.
