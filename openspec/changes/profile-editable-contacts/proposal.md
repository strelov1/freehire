## Why

Profile contacts and links are read-only parses of the uploaded CV, so wrong links (or a stuck extract) leave the candidate with no fix short of re-uploading — and while `structure_pending` is true, provisional reads strip summary, skills, and projects, so tailor bootstrap / stale-base refresh can open a CV missing those sections even when a superseded parse or the bank still knows them.

## What Changes

- Introduce **candidate-owned contacts** (name, email, phone, location, links) that the Profile tab can edit without a re-upload; extract only **fills empty** owned fields (hand edits win unless the user explicitly replaces from CV).
- Expose **parse status** on the résumé meta surface (`current` / `pending` / `failed` + short safe reason) with a **Retry parse** action; Profile shows this instead of an endless “still being parsed” state.
- Allow **editing project employments** (name, link, dates) on the Experience bank UI, using the existing employment write API.
- Fix **tailor / base seed composition** so summary, skills, and projects are not dropped when the structure stamp is pending: seed body sections from the current structure when stamped, otherwise from the last stored structured blob for those file-owned fields (while contacts come from owned contacts), and keep bank projects in `projects[]` as today.
- Update Profile UI: editable contacts + status/retry; Experience UI: edit project metadata.

## Capabilities

### New Capabilities

- `candidate-contacts`: Candidate-owned contact block, edit API, seed/heal/Profile consumers, extract fill-empty policy.

### Modified Capabilities

- `resume-structured-profile`: Parse status on résumé meta; Profile is no longer contacts-read-only-only; retry extract; seed may use superseded body sections when pending.
- `cv-tailoring`: Base/tailor seed and stale-base refresh MUST carry summary, skills, and projects when any of those exist on the seed composition (owned contacts + structure/bank), including while structure is pending.
- `experience-bank`: SPA (and wire clarity) for updating project employment fields the API already allows.

## Impact

- `internal/resume`, `internal/handler/resume.go`, `me_profile` / new contacts handlers, `cv_seed` / `bankedSeeder`, `cv_header_heal`, migrations for owned contacts + optional extract status
- `web` Profile + ExperienceBankView + API client
- `cmd/backfill-resume-structured` remains the ops reconciler; Retry reuses the same extract path
- No change to the stamp-equality rule for “current structure”; owned contacts and pending-body seed are the escape hatches
