## Context

The jobs filter modal (`FilterModal.svelte`) is a thin, job-specific wrapper over the
domain-agnostic `FilterModalShell.svelte`. The shell owns the chrome (header with
title/close, sectioned rail, deferred-apply footer); the wrapper supplies the job rail,
the `StagedFilters` edit store, and the pane controls. Selections are staged in memory
and only reach the live URL-synced list on **Show results** (`staged.commit(store)`).

Two pieces already exist and are reused wholesale:
- `profileStore` (`profile.svelte.ts`) — loads the signed-in user's `UserProfile`
  (`{ specializations, skills }`) once via `GET /api/v1/me/profile`, SSR-safe, null when
  signed-out or no profile. `save` enforces a non-empty skills set, so a loaded profile is
  never empty.
- The `specializations → category` / `skills → skills` mapping is already the convention:
  `/my/profile/+page.svelte` seeds a comparison filter with `append('category', spec)` and
  `append('skills', skill)`.

So this change is purely a new front-end affordance wiring those two together into the
staged store. No backend, API, or generated-contract work.

## Goals / Non-Goals

**Goals:**
- One-click application of the user's profile to the jobs filter modal, previewed before
  commit via the existing deferred-apply flow.
- Reset-then-seed semantics: applying the profile clears all staged filters, then stages
  `category` from specializations and `skills` from skills.
- Graceful states: create-profile link when signed-in without a profile; nothing when
  signed-out.
- Keep `FilterModalShell` domain-agnostic (no profile concept leaks into it).

**Non-Goals:**
- No backend/API/schema changes; no new profile fields.
- No merge/append mode — this change is reset-and-seed only (chosen deliberately below).
- No profile editing from the modal (that stays on `/my/profile`).
- No change to `CompanyFilterModal` or to profile-comparison reuses of `FilterModal`.

## Decisions

### Reset-then-seed, not merge
Applying the profile calls `staged.clear()` then stages the two facets, so the result is
exactly "the profile" regardless of prior staged state. Rationale: the profile expresses a
complete "what I'm looking for"; merging into arbitrary prior selections produces a
muddled, hard-to-reason-about result. The user still previews via **Show results** and can
hand-tweak before committing, so nothing is lost by resetting first.
_Alternative considered:_ merge/append — rejected as ambiguous and harder to undo.

### The action lives in the header, injected by the wrapper via a new shell snippet slot
`FilterModalShell` gains one optional `headerAction` snippet rendered in the header row.
`FilterModal` passes the profile button/link into it. Rationale: the profile concept is
job-specific and must not enter the shell; a single optional snippet is the minimal,
already-established extension pattern (the shell already takes `extra`/`footerNote`
snippets). `CompanyFilterModal` omits the slot and is unaffected.
_Alternatives considered:_ (a) a rail tab like "My filters" — heavier, and the header is
where a global "seed everything" action reads more naturally than a per-facet pane; (b) a
chip on the `/jobs` page outside the modal — bypasses the preview step the deferred-apply
contract is built around.

### Visibility gating in the wrapper
The action is shown only when `hasSavedTab`-style scope holds (full job modal:
`savedSearches && !railKeys`) AND the user is signed in. Within that: profile present →
**Apply my profile** button; signed-in but no profile → a `/my/profile` create link;
signed-out → render nothing. `profileStore.ensureLoaded()` is called when the modal opens
for a signed-in user (mirrors the existing Telegram-flag warm-up effect).

### Seeding mechanism
Add a small method to `StagedFilters`, e.g. `applyProfile(specializations, skills)`, that
does `clear()` then stages each value through the existing `facetAdd`/`add` path for
`category` and `skills`. Keeping it on the staged store (rather than inline in the
component) keeps the component declarative and makes the reset+seed unit-testable without a
DOM — consistent with how `apply(query)`/`clear()` already live there.

## Risks / Trade-offs

- **Stale profile in a long-lived tab** → `profileStore` caches after first load; if the
  user edits their profile elsewhere the modal could seed stale values. Mitigation:
  acceptable for MVP — the profile editor and the modal are the same SPA session and
  `profileStore` updates in place on save; a full staleness story is out of scope.
- **A specialization value no longer in the category vocabulary** → it would stage as an
  unmatched `category` value. Mitigation: profiles are saved from the same category
  vocabulary the facet uses, so this is not expected; the facet control simply shows the
  value as selected, and Show-results counts reflect reality.
- **Header real-estate on narrow screens** → one extra control next to the title.
  Mitigation: it's a compact button/link; follow existing header responsive styling.
