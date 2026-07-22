## Why

When a user's CV is sent to an LLM (fit analysis and structured extraction), real direct
identifiers — name, email, phone, home address, personal links — travel through our
self-hosted litellm gateway to external model providers. They are not needed to judge fit,
and they must never leave our perimeter. Deterministic detection alone cannot reliably find
the name (a spike showed it fails on multi-column CVs where the surname lives only inside the
email/URL); a local, context-aware PII model can, and was spike-VALIDATED.

## What Changes

- New `internal/pii` package: a high-precision regex floor (email / phone with a `YYYY-YYYY`
  date guard / URL / `@handle`) unioned with `PERSON`/`ADDRESS`/`LOCATION` spans from a local
  `openai/privacy-filter` detector, producing a `Redactor` with reversible numbered
  placeholders (`[REDACTED_NAME]`, `[REDACTED_EMAIL_1]`, …).
- New span-detection HTTP endpoint serving `openai/privacy-filter` (ONNX q4, CPU), co-located
  on the litellm box; `internal/pii` calls it. New env `PII_FILTER_URL`.
- `job-fit-analysis`: mask CV + structured résumé on the way into every stage prompt; restore
  only on the outbound emit + the returned/cached analysis; never restore data threaded into a
  later stage.
- `resume-structured-profile`: fill contact fields (name/email/phone/links) deterministically
  from detected spans; send only the redacted CV to the LLM for the semantic fields.
- **Fail-closed**: when the detector is unconfigured or unavailable, the CV is not sent to the
  LLM (fit analysis / structured extraction degrade to no-op, exactly like an unconfigured LLM).

## Capabilities

### New Capabilities
- `cv-pii-masking`: detect PII in CV text (regex floor ∪ local model spans) and produce a
  reversible `Redactor` that masks text into LLM prompts and restores it in user-facing output;
  fail-closed when the detector is unavailable.

### Modified Capabilities
- `job-fit-analysis`: CV and structured-résumé text sent to the LLM MUST be PII-masked, and
  user-facing output MUST be restored, without re-leaking PII into later stages.
- `resume-structured-profile`: contact fields MUST be derived from deterministic detection, and
  only redacted CV text may be sent to the LLM.

## Impact

- Code: new `internal/pii`; `internal/matchanalysis` (`AnalyzeStream`, `writeCV`,
  `writeStructured`, emit path); `internal/resumeextract` (`Extract`, prompt, contact fill);
  `internal/config` (`PII_FILTER_URL`).
- Infra: new privacy-filter span-detection service + ONNX q4 weights (~900MB) on the litellm
  box (`204.168.137.149`); ops health-check/monitoring. Deployed via `freehire-ops`.
- Behavior: fit analysis and structured extraction now hard-depend on the detector
  (fail-closed); `enrich` and all non-CV LLM traffic are untouched.
