## Context

See proposal.md — Why. Bank mutations update `experience_*` (and may merge skills into
`userprofile.skills`) but never push into `cvs.data`. The only whole-document apply from
seed today is `POST …/reset-from-resume` (tailored-only; reseeds base as a side effect) and
upload-stale base refresh on tailor bootstrap. Tailor History already uses `window.confirm`
before Reset. `ExperienceBankView` is shared by `/my/profile` and `/tailor/[slug]`.

Assumption (recorded): profile Skills tab edits do not prompt — CV skills come from the
structured résumé / keep-if-empty, not `userprofile.skills`.

## Goals / Non-Goals

**Goals:**

- Consent prompt after successful bank mutations on profile and tailor.
- Tailor agree → existing reset-from-résumé for the open CV.
- Profile agree → base reseed without opening a tailored CV.
- No silent rewrite; decline is a no-op.

**Non-Goals:**

- Prompting on profile Skills / specializations / contacts edits.
- Refreshing every tailored CV for every vacancy in one click.
- Auto-reset without consent.
- Changing seed composition or Align behaviour.
- Extension / assistant-driven bank writes (prompt is web UI after user-initiated save).

## Decisions

### D1 — Client-orchestrated prompt, reuse Reset

After `ExperienceBankView` reports a successful mutation, the host page shows a confirm
(same family as Reset copy). Tailor page passes the open `cvId` and calls
`api.resetCvFromResume`. No new "partial sync" API for experience rows.

**Why:** One apply path; History undo still works; matches product rule that open tailored
CVs are never rewritten behind the candidate's back.

### D2 — Base-only reseed for profile

Add cookie-only `POST /api/v1/me/cvs/base/reset-from-resume` (name bikeshed OK) that:
loads seed via `seedSource()`, 409 if unusable/`!hasSeedBody`, then
`reseedBaseFromSeed` / Create if absent — same `applySeedContent` as Reset.

**Why:** Today's Reset 409s on non-tailored; forcing profile users through a tailored id
is wrong. Alternatives: reset "most recent tailored" (surprising); no profile prompt
(leaves base stale).

### D3 — Prompt copy

Align with ArtifactPanel Reset: content from current seed (bank + résumé); layout/template
kept; History can undo on tailored. Profile copy: "Update your base CV from your experience
bank?" without claiming every tailored job CV was updated.

### D4 — Nag control

Prefer sessionStorage (or in-memory on the page) dismiss after "No" for the rest of the
tab session so rapid successive atom edits are not a confirm storm. "Yes" always allowed
again after another mutation if desired; or suppress until next navigation — pick
session dismiss after No in implementation.

### D5 — Where the hook lives

Shared helper used by both hosts after bank success callbacks, so profile and tailor cannot
drift. Prefer not burying HTTP inside `ExperienceBankView` without a host-supplied
`onRefreshOffer` — host knows whether a tailored id or base reseed applies.

## Risks / Trade-offs

- **[Risk] Confirm storm on bulk atom edits** → Mitigation: session dismiss after No;
  optionally debounce offer to once per burst.
- **[Risk] Candidate thinks profile Skills were applied to the CV** → Mitigation: do not
  prompt on Skills tab; copy names experience bank / résumé seed.
- **[Risk] Base reseed without tailored context surprises later tailor opens** → Mitigation:
  intended; next tailor bootstrap copies from refreshed base when minting new vacancies;
  existing tailored rows still need their own Reset (out of scope to fan-out).
- **[Risk] Reset is destructive on tailored** → Mitigation: same confirm gravity as History
  Reset; revision history remains.

## Migration Plan

Ship web prompt + optional base endpoint together. Rollback: remove prompt; leave Reset as
manual. No schema migration.

## Open Questions

None for planning — Skills-tab prompt deferred; multi-tailored fan-out deferred.
