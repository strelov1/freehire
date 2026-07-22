# PII masking for CV in LLM calls — design

**Date:** 2026-07-22
**Status:** design (awaiting review)
**Scope:** `internal/pii` (new), `internal/matchanalysis`, `internal/resumeextract`, plus a local PII-model sidecar.

## Goal & threat model

When a user's CV is sent to an LLM, real direct identifiers must **never leave our
perimeter and reach the model provider**. Our LLM traffic flows through a self-hosted
litellm proxy, but the proxy forwards to external model providers (OpenAI/Anthropic/…),
so masking must happen **before** the request leaves us.

The analysis output is served back to the **same** user (it is their own CV), so within
our system the data is not sensitive — the concern is strictly the outbound provider hop.

**PII in scope (mask):** full name, email, phone, home/postal address, personal links
(portfolio/LinkedIn/GitHub/Telegram handle).
**Explicitly out of scope (keep visible to the model):** employer names, universities,
job titles, skills, city/country context — these are load-bearing for fit scoring
(`experience_relevance`, `company_context`, `location_fit`) and are not direct identifiers.

## Spike findings (2026-07-22) that shaped this design

Ran the deterministic detectors against two real CVs (single-column and two-column):

- **Regex layer (email / phone / URL / @handle) — VALIDATED.** Caught the crisp
  identifiers on both layouts. One fix required: the phone regex matched a `YYYY-YYYY`
  date range as a phone number — needs a date-range guard.
- **Plain-text name detection by "top-of-CV header" heuristic — INVALIDATED for
  multi-column layouts.** `pdftotext -layout` puts a section header (`ABOUT ME`) on the
  first line and pushes the visible name far down; worse, the full surname often appears
  **only inside the email local-part / URL slug** (`alexbessmelcev`, `alex-bessmelcev`),
  never as plain text. A regex/heuristic cannot reliably recover the name.

**Consequence:** robust name/address detection needs a model that reads context. We use
a **local** PII model (never leaves our perimeter) for `PERSON`/`ADDRESS`/`LOCATION`
spans, with the regex layer kept as a high-precision floor.

## Architecture

### New package `internal/pii`

Pure orchestration + the deterministic detectors; the model call is behind a small
client interface so `internal/pii` stays testable without the sidecar.

```
type Contacts struct{ FullName, Email, Phone string; Links []string } // known, authoritative

type Span struct{ Start, End int; Kind string } // NAME|EMAIL|PHONE|LINK|ADDRESS

type Detector interface { Detect(ctx, text string) ([]Span, error) } // model sidecar impl

type Redactor struct { /* value->placeholder and placeholder->value maps */ }

func Build(ctx, text string, known Contacts, d Detector) (*Redactor, error)
func (r *Redactor) Redact(text string) string   // mask on the way INTO a prompt
func (r *Redactor) Restore(text string) string  // unmask on the way OUT to the user
```

- **Regex detectors (in-process, always run):** email, phone (with `YYYY-YYYY` date
  guard), URL (http/https, bare `domain.tld/…`, `linkedin.com/…`, `github.com/…`,
  `t.me/…`), `@handle`.
- **Model detector (sidecar):** OpenAI Privacy Filter returns token-level PII spans;
  we keep `PERSON`/`ADDRESS`/`LOCATION`(home) and drop categories we deliberately allow.
- **Merge:** regex spans ∪ model spans → one span set → the `Redactor`. Regex insures the
  crisp identifiers; the model adds name/address the regex cannot.
- **Placeholders** are numbered and reversible: `[REDACTED_NAME]`, `[REDACTED_EMAIL_1]`,
  `[REDACTED_PHONE_1]`, `[REDACTED_LINK_2]`, `[REDACTED_ADDRESS]`. Replacement is on
  word boundaries; known/full-value matches take priority over short single tokens to
  bound over-redaction.

### Local PII-model sidecar

- **Model:** `openai/privacy-filter` (Apache-2.0, 1.5B MoE, ~50M active params, CPU-OK,
  ~96% F1, 128K context). Pulled **only** from the official repo (a malicious typosquat
  existed).
- **Shape:** a small HTTP service (Python) co-located with the backend; `internal/pii`'s
  model `Detector` calls it over localhost. One call per CV per analysis / per upload —
  not per stage.
- **Configuration:** a new env (e.g. `PII_FILTER_URL`) on the server/worker config. When
  unset the detector is considered unconfigured.

## Data-flow invariant (most important)

> **Mask on the way INTO every prompt; restore ONLY on the way OUT to the user. Data that
> is threaded into a later stage is NEVER restored** — otherwise PII re-leaks into
> Stage 2/3.

## Integration: `matchanalysis`

- At the top of `AnalyzeStream`, build a `Redactor` from `in.CVText` + `in.StructuredResume`
  (authoritative contacts) via `pii.Build`.
- `writeCV` and `writeStructured` pass their text through `Redactor.Redact` — the provider
  sees `[REDACTED_*]`.
- Wrap `emit` in a decorator that runs `Restore` over each outbound `Event`'s user-facing
  strings (requirements / dimensions / final) — on a **copy**, leaving the internal
  `reqs`/`verdict` (which feed Stage 2/3 prompts) masked.
- The final `Analysis` returned to the handler and cached is `Restore`d (it is the user's
  own data in their `user_job_analysis` row; storing real values there is correct).
- The handler is unchanged — masking is entirely internal to the chain.

## Integration: `resumeextract` (removes the upload-time leak)

Today the LLM extracts contacts, which leaks them on upload. New flow:

- Build a `Redactor` from the CV via `pii.Build` (regex + model).
- Fill `Structured.FullName/Email/Phone/Links` from the **detected** identifiers, not from
  the model's answer.
- Send the **redacted** CV to the LLM only for the semantic fields (summary, experience,
  education, skills — no PII there; employer/university names remain visible). Adjust the
  prompt: "contacts are provided separately, do not extract them."
- Because `Structured` now carries the real contacts, `matchanalysis.writeStructured` will
  re-mask them with the same `Redactor` (same string → same placeholder) — consistent.

## Failure mode: **fail-closed**

If the sidecar is unconfigured or unavailable, `pii.Build` returns an error and we do **not**
send the CV to the LLM:

- `matchanalysis`: no analysis produced (same best-effort degradation as an unconfigured
  LLM — the deterministic `jobmatch` bar is untouched).
- `resumeextract`: behaves like `ErrDisabled` — upload, embedding, and deterministic
  extractors are untouched; no structured résumé is produced this run.

This preserves the "provider never sees PII" guarantee strictly, at the cost of the
LLM features when masking is unavailable. The regex layer alone is **not** treated as
sufficient for the name, so regex-only is not a fallback.

## Testing

- `internal/pii` (no sidecar): table tests over email/phone/URL/@handle, `YYYY-YYYY`
  phone guard, numbered multi-value placeholders, word-boundary replacement,
  `Restore(Redact(x))` round-trip, over-redaction guard. Model `Detector` is faked.
- `matchanalysis`: assert known PII never appears in the Stage 1/2/3 prompt strings, and
  that PII is restored in emitted + returned output; assert fail-closed on a failing
  detector.
- `resumeextract`: assert contacts are filled from detection and that the LLM input
  carries no PII; assert fail-closed.
- **Implementation-time model spike:** run the actual Privacy Filter against the two spike
  CVs and confirm it recovers the buried surname (`Bessmelcev`) and the `Alex` name before
  wiring it in.

## Known trade-offs / seams

- **Over-redaction:** a name equal to a common word could touch the body; mitigated by
  word-boundary + full/known-value priority. Restore fixes output readability but not an
  input-semantics hit — accepted, monitored.
- **Address:** modern CVs rarely carry a postal address (only a city, which we keep). The
  model covers the rare real address; no separate deterministic address detector.
- **Sidecar is new infra:** one Python service + weights to deploy and monitor. Deployment
  host (co-locate on the backend host vs beside the litellm proxy) is an ops decision to
  settle during implementation; CPU inference is sufficient.
