## Context

CV text reaches the LLM in two places over the shared `internal/llm` client: the
`matchanalysis` three-stage fit chain (raw CV + structured-résumé JSON, in every stage
prompt, streamed over SSE) and the `resumeextract` upload-time extraction (whole CV). Both
flow through the self-hosted litellm gateway (`204.168.137.149`, the py `:4000` proxy — the
`litellm-rust` `:4001` path has no guardrails), which forwards to external providers.

Two spikes shaped this design: (1) a deterministic-only detector VALIDATED regex for
email/phone/URL/`@handle` but was INVALIDATED for names on multi-column CVs (the surname
lives only inside the email/URL); (2) the real `openai/privacy-filter` (ONNX q4, CPU) was
VALIDATED — it recovered the hidden surname, ran <2.5s/CV, and did not over-redact employers
or cities. Full detail: `docs/superpowers/specs/2026-07-22-pii-masking-cv-llm-design.md`.

## Goals / Non-Goals

**Goals:**
- Direct identifiers (name, email, phone, home address, personal links) never leave our
  perimeter to the model provider.
- User-facing analysis output shows the real values (restore), and PII never re-leaks into a
  later stage's prompt.
- Surgical scope: only CV-bearing calls are masked; `enrich` of public job text is untouched.

**Non-Goals:**
- Masking employer/university/title/skill/city context (needed for fit scoring).
- A litellm masking guardrail (evaluated and rejected — see Decisions).
- Reversible masking of anything other than the detected CV PII.

## Decisions

**D1 — Detection = regex floor ∪ local model.** Regex is high-precision for the crisp
identifiers and free; the model adds name/address the regex cannot. Union both into one span
set. *Alternative:* model-only — rejected: regex insures the crisp identifiers if the model
misses and needs no weights.

**D2 — Model served as a span-detection HTTP endpoint (form B), NOT a litellm guardrail.**
`resumeextract` must *obtain* contacts; a proxy guardrail masks the CV so the LLM cannot
return them and does not hand the detected spans back — so a callable detector is required
regardless, and once it exists `matchanalysis` uses it too, making the guardrail redundant.
In-Go masking keeps scope surgical (no per-request guardrail opt-in), keeps restore under our
control (litellm `output_parse_pii` has documented streaming-restore bugs; `matchanalysis` is
SSE), and keeps freehire's privacy self-contained. The weights live on the litellm box; the
endpoint is off the proxy request path. *Alternatives:* guardrail-only (breaks resumeextract,
streaming risk); A+B hybrid (two mechanisms) — rejected for surface area.

**D3 — Mask-in / restore-out invariant.** `internal/pii.Redactor` masks text into every stage
prompt; a decorator around `emit` restores a copy of each outbound event, and the returned +
cached analysis is restored. Internal `reqs`/`verdict` threaded into later stages stay masked.

**D4 — Fail-closed.** No detector ⇒ no CV to the LLM. `matchanalysis` degrades to no analysis
(like an unconfigured LLM); `resumeextract` behaves like `ErrDisabled`. Regex-only is not an
accepted fallback for the name.

**D5 — Model = `openai/privacy-filter` (ONNX q4, onnxruntime, CPU).** Purpose-built, Apache-2.0,
BIOES over 8 PII types, ~96% F1, 128K context, <2.5s/CV on CPU. Pulled only from the official
repo (a typosquat existed). Production uses the shipped Viterbi decoder for span stitching.

## Risks / Trade-offs

- **Over-redaction** (a name equal to a common word) → word-boundary + full/known-value
  priority; spike showed no employer/city false positives.
- **Bare first-name missed** (e.g. standalone "Alex") → accepted: weak identifier, and it is
  covered by the person/email/handle spans that ARE caught.
- **New infra hard-dependency** (detector on the CV path, fail-closed) → health-check and
  monitor it like the other model-path infra; an outage disables fit analysis + structured
  extraction (but not uploads, embeddings, deterministic extractors, or enrich).
- **Detector latency on the CV path** (<2.5s/CV) → one call per analysis/upload, not per
  stage; acceptable against the multi-stage LLM cost.
- **resumeextract prompt change** (contacts no longer LLM-extracted) → the merge fills
  contacts from detection; the LLM prompt is told contacts are handled separately.

## Migration Plan

1. Deploy the privacy-filter span-detection endpoint + ONNX q4 weights on the litellm box via
   `freehire-ops`; set `PII_FILTER_URL` on the server and the resume/embed worker.
2. Ship `internal/pii` + the two integrations. Until `PII_FILTER_URL` is set, fail-closed
   leaves fit analysis and structured extraction as no-ops (safe default; no PII leak).
3. Rollback: unset `PII_FILTER_URL` (fail-closed no-op) or revert the code; no schema change.
