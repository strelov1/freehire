## Why

Resetting the base CV from a résumé (triggered automatically after an experience-bank edit, via the "update your base CV?" prompt on `/my/profile/experience`) can be refused for a specific, well-understood reason — most commonly `cvedit.ErrListCap`, when reseeding would exceed the per-role bullet ceiling. The backend already reports exactly which role hit the ceiling and reassures the candidate that nothing was lost. The frontend discards that message and always shows one generic, unhelpful string instead, confirmed on production (`POST /api/v1/me/cvs/base/reset-from-resume` → 409 with a specific reason, UI showing the generic fallback regardless).

## What Changes

- `web/src/routes/my/profile/experience/+page.svelte`'s `offerRefreshAfterBankEdit` now surfaces the caught error's own message (via the codebase's existing `errorMessage(e, fallback)` helper, already used elsewhere in this same feature area) instead of a hardcoded string, falling back to the generic message only when the failure carries no useful message of its own (e.g. a raw network failure).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `cv-builder`: resetting the base CV from a résumé must surface the server's specific refusal reason to the candidate when one is available, rather than a generic failure message.

## Impact

- Frontend only, one file: `web/src/routes/my/profile/experience/+page.svelte`. No backend changes — the backend's message was already correct and specific; only the frontend was discarding it.
