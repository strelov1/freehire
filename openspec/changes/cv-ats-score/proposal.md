## Why

The verdict page tells a user how their skills cover the market (market-coverage)
but says nothing about whether an ATS can even read their CV or whether it carries
the role's keywords — the two things that actually gate an application. Users want
a concrete, actionable "how ATS-ready is my CV?" answer. This adds that as a CV
readiness score + fix checklist next to market-coverage, built on the CV the
profile already stores and the now-fixed skill matcher.

Wording note: new user-facing copy and API paths use **"CV"**, not "résumé"
(existing shipped résumé-storage code is left as-is).

## What Changes

- **New `internal/atscheck` package (pure, deterministic).** `Score(cvText,
  roleTopSkills)` → `{overall, readability, keyword_match, checks[]}`, each check
  `{id, status: pass|warn|fail, label, fix}`. Deterministic checks over the plain
  CV text:
  1. **Machine-readable** — near-empty extracted text ⇒ scanned/image PDF ⇒ hard
     fail (the one ATS-killer we can reliably detect).
  2. **Contact info** — email + phone present.
  3. **Standard sections** — curated heading dictionary (Experience/Education/
     Skills, EN+RU).
  4. **Dates** — year / mm-yyyy patterns present.
  5. **Length** — word-count band (flag very short/long).
  6. **Bullets** — structured vs wall-of-text.
  7. **Keyword-match** — of the role's top-N in-demand skills (same Meili facets
     the verdict uses), how many appear as literal `skilltag` matches in the CV
     text (ATS keyword-scan simulation). Distinct from market-coverage, which
     scores the profile's skill SET vs the market.
  `overall` = weighted blend of readability + keyword_match (+ content_quality
  when the LLM layer ran); weights are tunable constants.
- **Optional LLM qualitative layer (nil-safe).** Over the CV text: weak/passive
  vs strong action verbs, achievement/quantified vs responsibility-only bullets,
  a garbled-text flag (soft multi-column signal — the extractor is plain-text, so
  multi-column/tables are NOT hard-detectable; stated honestly), and 2-3 concrete
  fixes. JSON-forced via `GenerateJSON` + Sanitize/Validate (mirrors the old
  `coherence.go`). No LLM configured/failed ⇒ deterministic-only score (200).
- **Re-add a nil-safe server LLM client** (the `config.Config` LLM/Langfuse fields
  and `cmd/server` construction removed in the verdict refactor), gated on `LLM_*`.
- **Endpoint** `GET/POST /me/profiles/:id/ats-report` (owner-scoped; 404/503 like
  verdict). GET = live deterministic report merged with any cached LLM review;
  POST = run the LLM review over the stored CV and cache it.
- **Cache the LLM review per-USER** keyed to the stored CV (role-independent, so
  computed once per CV and reused across profiles/roles), on the users CV pointer;
  invalidated when the CV is replaced. Raw CV text is never stored (existing
  invariant).
- **Web:** a "CV readiness" section on `/my/profiles/[id]/verdict` (score,
  sub-scores, checklist with fixes, "Run AI review" when LLM is on) plus a CV
  upload/replace control (the page has none today); `$lib/api`/`$lib/types` +
  regenerated contracts.

## Capabilities

### New Capabilities
- `cv-ats-score`: the CV ATS-readiness score — deterministic structural + keyword
  checks producing a score and fix checklist, plus an optional nil-safe LLM
  qualitative layer, served per profile against the caller's stored CV.

### Modified Capabilities
<!-- none at the requirement level: market-coverage (resume-verdict) is unchanged;
     this adds a new, separate report on the same page. -->

## Impact

- **Backend:** new `internal/atscheck` (pure) + its LLM analyzer; new
  `internal/handler/ats_report.go` (GET/POST) reusing the verdict's facet fetch;
  `config.Config` + `cmd/server` re-add the nil-safe LLM client; `internal/resume`
  or a repo method to read+cache the per-user CV analysis.
- **Database:** migration adding a per-user CV-analysis cache column (JSONB) to
  `users`; regen sqlc. No new Meili attribute ⇒ **no reindex**.
- **Frontend:** verdict page section + CV upload control, `$lib/api`,
  `$lib/types`, generated contracts.
- **Delivery:** large change — tasks are phased so the deterministic score (no
  LLM, no migration) is complete and shippable first, then the LLM layer (server
  client + cache migration + POST + AI-review UI) lands on top.
