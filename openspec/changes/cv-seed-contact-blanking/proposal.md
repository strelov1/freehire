## Why

When a candidate starts tailoring, the tailored CV often arrives with an empty header — no name, phone, location, email, or links — even though those values are on the uploaded résumé and were sometimes already typed into the base CV. Two independent defects compound: candidate residence never survives extraction, and a stale or absent structured résumé still counts as a usable seed because the experience bank alone makes `seedable` true, so bootstrap/reset overwrite a filled header with blanks.

## What Changes

- Restore the candidate's residence into the structured résumé from PII detection (`ADDRESS`), the same way name/email/phone/links are restored today, instead of asking the extraction LLM to invent it from a redacted CV.
- Treat a seed that lacks a current structured résumé (stamp mismatch / extraction pending / extract failed) as **not usable for whole-document replace**, even when the experience bank has history.
- When applying a seed onto an existing CV (stale-base refresh, reset-from-résumé), empty seed contact fields MUST NOT overwrite non-empty header fields already on the document.
- Keep agent-facing redaction unchanged: cookie/PDF still show the real header; API-key / `cv_get` still withhold identifying contacts.

## Capabilities

### New Capabilities

- (none)

### Modified Capabilities

- `resume-structured-profile`: contact restoration grows to include the candidate's location from deterministic detection; location is no longer an LLM-only field that disappears when the residence line is redacted.
- `cv-pii-masking`: `Contacts` recovered by the redactor include location from `ADDRESS` spans (first plausible value), parallel to name/email/phone/links.
- `cv-tailoring`: bootstrap freshness refresh and reset-from-résumé must not blank the contact header; a bank-only / structure-absent composition is not a usable whole-document seed.

## Impact

- `internal/pii` — `Contacts` + `fillContact` for `ADDRESS` → location.
- `internal/resumeextract` — restore `Location` from `red.Contacts()` alongside the other contact fields; schema/prompt wording that still treats location as model-filled may need tightening so the model is not asked to invent a redacted residence.
- `internal/handler/cv_seed.go` / `cv_seed_apply.go` / `cv_reset.go` / `cv_tailor.go` — seedability gate and non-blanking header merge on apply.
- Tests: extraction contact round-trip with an ADDRESS span; seed/reset/reseed cases where bank has experience but structure is stale/absent; apply-seed preserves an existing header when the seed's contacts are empty.
- No migration. Existing `resume_structured` rows with a null location self-heal on the next upload; blanked CV headers need reset-from-résumé (after the fix) or a manual edit — no automatic rewrite of existing `cvs` rows.
