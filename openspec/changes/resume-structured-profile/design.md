## Context

A stored résumé today is opaque and re-parsed per fit analysis:

- `internal/resume` stores the original file in S3 under a per-user key, records a pointer (`users.resume_object_key` + `resume_uploaded_at`), and derives plain text on read (`Store.Text` — PDF parse or UTF-8, never persisted).
- Two upload paths — `PutResume` (`PUT /me/resume`) and `ExtractResumeProfile` (`POST /me/resume/extract`) — both call `Store.Put` and then kick a **background best-effort** `go a.embedResume(userID, text)` (CV vector for `/my/recommendations`). This goroutine pattern is the template this change follows.
- `internal/jobfit` runs the three-stage fit chain over `Input.CVText` (raw text) plus the deterministic `jobmatch` anchor and job geography.
- Existing résumé extractors are deterministic and LLM-free: `cv-autofill` (dictionary skills/seniority/categories for onboarding) and `resume-skill-extraction` (skilltag slugs). They stay as-is.
- The handler already holds an `*llm.Client` (`cfg.LLM`), used by `jobfit.NewAnalyzer(cfg.LLM.WithTimeout(...))`. `llm.Client.GenerateJSON` is the JSON-mode entry point. `enrich`/`jobfit` establish the "sanitize to a typed, bounded, controlled contract before persist" discipline.

## Goals / Non-Goals

**Goals:**
- Derive a typed, sanitized **structured résumé** (contacts, summary, work experience with dates, education, languages, links, total years) from the uploaded CV, once, best-effort via the LLM.
- Persist it read-only per user, provenance-stamped, self-healing across re-uploads (never describe a different CV than the one currently stored).
- Serve it on the résumé read surface for a read-only profile rendering.
- Feed it into the fit chain as pre-normalized context beside the existing CV text, degrading to today's behavior when absent.

**Non-Goals:**
- Per-field editing / user correction of the structured résumé (read-only this change).
- Replacing the deterministic extractors (`cv-autofill`, `resume-skill-extraction`) or the raw-text fit input — the structure is additive.
- A backfill worker that re-extracts existing résumés or re-extracts on an `LLM_MODEL` upgrade (the model stamp is stored so a future backfill can find stale rows; the worker itself is a noted seam, not built now).
- Multi-résumé per user; extraction of anything not present in the CV.

## Decisions

### 1. Extraction lives in a dedicated `internal/resumeextract` package, LLM behind the shared client

A new package owns the typed `Structured` contract, the prompt, and `Structured.Sanitize()`. `Extractor` wraps `*llm.Client`; `Extract(ctx, cvText) (Structured, error)` calls `GenerateJSON` and sanitizes. Rationale: mirrors `internal/jobfit`/`internal/enrich` — a self-contained, typed, cacheable prompt unit — and keeps `internal/resume` (S3 + pointer persistence) free of LLM coupling. *Alternative rejected:* a method on `resume.Store` — would couple object storage to the LLM and the controlled-vocabulary contract.

`Sanitize` is the persist guard and the prompt-injection guard for the untrusted CV text (same invariant as enrich/jobfit): bound every string length, cap array cardinalities (e.g. experience/education/languages), coerce `total_years` into a sane non-negative range, drop empty entries. The model never introduces an out-of-bounds value that reaches storage.

### 2. Handler orchestrates a background goroutine, mirroring `embedResume`

Both upload paths gain `go a.extractStructuredResume(userID, up.Text)` beside the existing `go a.embedResume(...)`. It runs on its own `context.WithTimeout(context.Background(), …)` (the request context is gone once the upload responded), extracts, and persists via `resume.Store`. Best-effort: unconfigured LLM (`cfg.LLM == nil` / analyzer disabled) or any error is logged (never the CV bytes) and swallowed — upload, embedding, and the deterministic extractors are untouched.

### 3. Storage: one JSONB + a model stamp on `users`, stamped with the résumé it describes

Migration `0011_resume_structured.sql` adds `users.resume_structured jsonb`, `users.resume_structured_model text`, and `users.resume_structured_uploaded_at timestamptz` (all nullable; NULL = unextracted). The persist writes the sanitized JSON, the producing `model` id, and **the current `resume_uploaded_at`** as the stamp.

**Self-healing staleness (no synchronous clear on re-upload):** the read surface returns the structured résumé only when `resume_structured_uploaded_at == resume_uploaded_at`; a mismatch (a newer CV whose extraction has not landed yet, or a persistent extraction outage) is treated as *absent*, so the profile never shows a structure derived from a different CV. On success the stamp matches again and it reappears — self-healing within the seconds the background extraction takes. Rationale: this is exactly `jobfit`'s stamp-and-compare freshness discipline; it needs no synchronous clear in the hot upload path and cannot flash the wrong CV's data. *Alternative rejected:* clear the column synchronously on re-upload — adds work to the upload path and briefly blanks even when the CV is unchanged content.

`Delete` clears all three columns alongside the pointer (the structure must not outlive the résumé).

### 4. Read surface: extend the résumé status response

`GET /api/v1/me/resume` (`GetResume`, cookie-only) is where the profile already reads résumé state; extend `resumeMetaResponse` with an optional `structured` field (the typed `Structured`, or null when absent/stale/unextracted). *Alternative rejected:* a separate `GET /me/resume/structured` — an extra round-trip for the same page load with no isolation benefit.

### 5. Fit-analysis: an additive, pre-normalized input

`jobfit.Input` gains `StructuredResume string` (the sanitized JSON, empty when absent). The Stage-1 prompt receives it as pre-normalized candidate context **beside** `CVText`, never replacing it — so a missing extraction degrades to exactly today's text-only analysis. The handler (`PostJobFit`, `job_fit_stream.go`) loads the stored structured résumé when building `Input`. Because the raw text remains the ground truth, this cannot lower analysis quality; it can only add signal.

### 6. Contract to TypeScript via `cmd/gen-contracts`

Register `resumeextract.Structured` in `cmd/gen-contracts` so the SPA renders against a generated type, mirroring `jobfit.Analysis`.

## Risks / Trade-offs

- **Extra LLM call per upload** → best-effort and off the response path (background goroutine); no new required config — skipped entirely when `LLM_*` is unset, exactly like every other LLM feature.
- **Stale window during re-upload / LLM outage** → the upload-time stamp makes a mismatch read as *absent* rather than showing the wrong CV's structure; it self-heals on the next successful extraction.
- **LLM hallucination / prompt injection from untrusted CV text** → `Structured.Sanitize` bounds and coerces every field to the typed contract before persist or serve (same guard as `enrich`/`jobfit`); the model cannot persist an out-of-bounds value.
- **Two upload paths must both wire the goroutine** → covered by tests on both handlers; the goroutine is a one-liner beside the existing `embedResume` call, so drift is unlikely.
- **New migration on a persistent DB** → `0011` must be applied to prod manually before deploy (per the migrations gotcha); columns are additive and nullable, so an unapplied migration degrades reads, it does not corrupt.
