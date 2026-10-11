## Why

GitHub issue #2130 blames "the PII filter" for interfering with tailoring. Investigation (prod
DB query, 2026-10-10) found the real mechanism: background structured-résumé extraction — the
step that seeds the experience bank and the base CV — fails for roughly 4-8% of uploads every
day (358 affected users in the sample queried), for several reasons (LLM timeout, LLM-provider
rate limits/billing, and the PII detector's own fail-closed design among them). The backend
already tracks this precisely (`users.resume_extract_status`/`resume_extract_detail`, exposed on
`GET /me/resume` as `parse_status`/`parse_detail`), and a code comment in the onboarding wizard
admits outright that "a third of uploads" time out — this is a known, not a hypothetical, failure
rate. The onboarding wizard already shows a "failed" state to the candidate there. Nowhere else
does: `grep` across `web/src` shows `parse_status` read in exactly one file. A candidate whose
extraction failed — during onboarding and missed, or on any later re-upload — opens the
Experience tab (profile or Tailor) to a permanently empty bank with no explanation and no way to
fix it, which is the "architecture limitation" the issue's own comment gestures at.

## What Changes

- Surface the already-tracked `parse_status`/`parse_detail` on the experience bank view
  (`ExperienceBankView.svelte`, shared by `/my/profile/experience` and the Tailor workspace's
  Experience tab) as a dismissable-by-success banner, mirroring the onboarding wizard's existing
  wording for the `failed` state.
- Add a **retry** action to that banner: a new `POST /me/resume/retry-extract` endpoint that
  re-derives the structured résumé from the ALREADY-STORED upload — no re-upload required. It is
  a thin handler over three methods that already exist (`Store.Status`, `Store.Text`,
  `deriveResumeArtifacts`), not new storage-layer code.
- No change to the extraction's own fail-closed design (PII-detector-unavailable, LLM-unconfigured,
  LLM-error) — those stay exactly as `resume-structured-profile`'s existing "Extraction failure is
  swallowed" requirement already specifies. This change is purely about visibility and giving the
  candidate a way to ask for another attempt, not about changing when extraction succeeds.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `resume-structured-profile`: gains a requirement that a failed structured-résumé extraction can
  be retried on demand, from the already-stored upload, without the candidate re-submitting their
  file.
- `experience-bank`: gains a requirement that the bank view surfaces a failed (or still-pending)
  résumé extraction with an explanation and a retry action, alongside the existing
  unconfirmed-achievements banner it already shows.

## Impact

- Backend: `internal/api/handler/resume.go` (new `POST /me/resume/retry-extract` handler, built
  entirely from `Store.Status`, `Store.Text`, and the existing `deriveResumeArtifacts`). No
  change to `internal/candidate/resume/resume.go` itself.
- Frontend: `web/src/lib/components/ExperienceBankView.svelte` (fetch `GET /me/resume` status,
  render the banner + retry button), `web/src/lib/api.ts` (new `retryResumeExtract()` call).
- Unaffected: the onboarding wizard's own existing handling of `parse_status` (left as-is, matched
  for wording where sensible but not refactored to share code — it has a materially different
  job: driving a one-time wait loop, not a persistent banner).
- Unaffected: `internal/candidate/pii/*`, `internal/candidate/resumeextract/*` — no change to
  detection, redaction, or extraction logic itself, only to what happens with an already-known
  failure.
