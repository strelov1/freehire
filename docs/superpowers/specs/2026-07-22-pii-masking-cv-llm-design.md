# PII masking for CV in LLM calls — design

**Date:** 2026-07-22
**Status:** design — spike VALIDATED, ready to plan
**Scope:** `internal/pii` (new), `internal/matchanalysis`, `internal/resumeextract`, plus a co-located `openai/privacy-filter` span-detection endpoint.

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
  **only inside the email local-part / URL slug** (`jordanprice`, `jordan-price`),
  never as plain text. A regex/heuristic cannot reliably recover the name.

**Consequence:** robust name/address detection needs a model that reads context. We use
a **local** PII model (never leaves our perimeter) for `PERSON`/`ADDRESS`/`LOCATION`
spans, with the regex layer kept as a high-precision floor.

### Model spike (2026-07-22) — VALIDATED

Ran the real `openai/privacy-filter` (ONNX **q4**, `onnxruntime` on CPU, no torch) against
the same two CVs. Labels are BIOES over 8 types (`private_person/email/phone/url/address`,
`account_number`, `secret`, `private_date`).

- **Ilya (single-column, 1542 tok, 2.2s):** `private_person: Ada Lovelace`, email, github/linkedin URLs.
- **Alex (two-column, 1166 tok, 0.9s):** recovered the hidden surname — `private_person:
  jordan-price`, `/jordan-price`, `@jprice_dev`; email + portfolio URL. (Bare
  first-name "Alex" alone was not tagged — a weak identifier, and it is covered by the
  caught spans anyway.)
- **No over-redaction:** employers (RingCentral, Informa, emcd.io), universities, and
  cities (London, Belmont, Florianópolis) were left untouched on both CVs — exactly the
  context the fit analysis needs stays visible.

Verdict: **VALIDATED** — fast on CPU (<2.5s/CV, one call per analysis/upload), recovers the
name the deterministic layer could not, and does not touch employer/geo context. Production
uses the shipped Viterbi decoder (`viterbi_calibration.json`) for span stitching; the spike
used plain argmax and still succeeded.

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

### Privacy-filter detection endpoint (integration form B)

Decision: the model is served as a **span-detection HTTP endpoint**, and freehire does the
masking/restoring in Go — NOT a litellm masking guardrail. Rationale:

- `resumeextract`'s whole job is to *obtain* contacts; a proxy guardrail masks the CV so the
  LLM can't return them and does not hand the detected spans back to freehire — so a callable
  detector is needed regardless. Once it exists, `matchanalysis` uses it too, and the proxy
  guardrail becomes redundant.
- In-Go masking keeps the scope surgical (CV only — `enrich` of public job text is never
  touched, no per-request guardrail opt-in), keeps restore fully under our control (litellm's
  `output_parse_pii` has documented streaming-restore bugs and `matchanalysis` is SSE), and
  keeps freehire's privacy self-contained rather than coupled to the litellm repo/deploy.

- **Model:** `openai/privacy-filter` (Apache-2.0, ONNX q4, `onnxruntime`, CPU-OK, ~96% F1,
  128K context). Pulled **only** from the official repo (a malicious typosquat existed).
- **Shape:** a small HTTP detection service that returns PII spans (`{start,end,kind}`) for a
  text. Co-located on the litellm box (`204.168.137.149`) so the weights "live" beside the
  gateway, but freehire calls it directly — it is NOT on the litellm proxy request path.
  `internal/pii`'s model `Detector` is the HTTP client. One call per CV per analysis / upload.
- **Configuration:** a new env (e.g. `PII_FILTER_URL`) on the server/worker config. When
  unset the detector is considered unconfigured (→ fail-closed, below).

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
- Model already spiked and VALIDATED (see above); wiring tests use a faked `Detector`.

## Known trade-offs / seams

- **Over-redaction:** a name equal to a common word could touch the body; mitigated by
  word-boundary + full/known-value priority. Restore fixes output readability but not an
  input-semantics hit — accepted, monitored.
- **Address:** modern CVs rarely carry a postal address (only a city, which we keep). The
  model covers the rare real address; no separate deterministic address detector.
- **Detection endpoint is new infra:** one small HTTP service + ONNX q4 weights (~900MB),
  co-located on the litellm box (`204.168.137.149`), CPU inference (<2.5s/CV). It serves
  span detection only; it is not on the litellm proxy request path. To monitor and health-
  check like the other model-path infra.
- **Streaming restore is NOT a concern here** (unlike the litellm-guardrail form): restore
  runs in Go on the emitted/returned analysis, so the SSE path is unaffected.
