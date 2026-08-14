## Context

Full write-up: `docs/superpowers/specs/2026-08-14-profile-alert-toggle-design.md`.
This document nails down the exact API/DB shape against the real code
(`internal/savedsearch`, `internal/handler/me_searches.go`,
`web/src/lib/facetModel.ts`, `web/src/lib/components/ProfileForm.svelte`).

## Goals / Non-Goals

**Goals:**
- One toggle, backed entirely by the existing saved-search + subscription
  primitives — no new matching engine, no server-side re-implementation of
  "profile → filters."
- The profile-derived search stays current: a profile edit updates its
  filters without the user doing anything.

**Non-Goals:**
- A skill-coverage-threshold notification engine (explicitly deferred to a
  future change if ever wanted).
- A dedicated channel-picker in the toggle — it inherits the account's
  existing notification channel default.

## Decisions

- **`filtersFromProfile()` stays client-side; the server never re-derives
  filters from a profile.** Verified against
  `web/src/lib/facetModel.ts:213` — it already flattens location
  preferences, skill include/exclude overlap, and specializations into a
  `JobFilters`, which `toSearchString()` turns into the canonical query
  string every saved search stores. Porting this to Go would risk the two
  copies disagreeing about what "my profile, as a search" means. The
  toggle's create/sync calls run this exact function and POST/PATCH the
  result through `internal/savedsearch`'s existing API — unchanged from
  what "Apply my profile" already produces today.
- **`derived_from_profile` is a plain boolean on `saved_searches`, set only
  at create time.** `internal/savedsearch.Service.Create(ctx, userID, name,
  query string)` gains a `derivedFromProfile bool` parameter; `Update`
  (rename/re-query) is untouched — it already accepts an arbitrary query,
  which is exactly what the sync-on-save call needs, so no new "sync"
  endpoint exists.
- **The at-most-one invariant is enforced by a partial unique index, not
  application-level locking.** `CREATE UNIQUE INDEX ... ON saved_searches
  (user_id) WHERE derived_from_profile` — a second `Create` with the flag
  set hits the unique violation, mapped to a new `ErrProfileSearchExists`
  (409), mirroring how `ErrDuplicateName` already maps a name-collision
  violation. The client is expected to check its own already-loaded
  `savedSearches` store before calling create (avoiding the round trip in
  the common case), so this 409 path is a race guard, not the primary flow.
- **The subscription always goes out on the email channel, not the
  account's preferred one.** The original design reused
  `notification_settings.channels[0]`, but that array can list telegram or
  push before the user ever links them — a code-review finding caught that
  this created a silently undeliverable alert with no error surfaced.
  Email needs no linking step, so it is always deliverable; the user can add
  another channel from the same Search alerts page afterward. If the
  subscribe call still fails, the just-created search is deleted so the
  toggle never ends up "on" for a search nothing is subscribed to.
- **Deleting the search is the disable path — no soft "paused" state.**
  Matches `saved-searches`' existing `DELETE /me/searches/:id` semantics
  exactly (cascades the subscription via the FK already in place for manual
  deletes). Re-enabling later is a fresh `Create`, not a resurrect.

## Web flow

- `/my/notifications/searches` (`SavedSearchesView.svelte`, right after the
  Telegram connection card) renders the toggle, gated on the account having
  a candidate profile (`profileStore.profile`) — nothing to derive filters
  from otherwise. Its `on` state is
  `savedSearches.items.some(s => s.derived_from_profile)` — no separate
  fetch, since both stores are already loaded for the page's other uses.
  (Originally placed on `/my/profile`'s Settings tab; moved here per
  user feedback post-ship — Search alerts is where every other
  saved-search/subscription control already lives, and a second copy on the
  profile page would need to stay in permanent sync with this one.)
- **Enable**: `filtersFromProfile(profile)` → `filtersToParams(...).toString()` → `api.createSavedSearch(name, query, true)`
  → `api.createSubscription(search.id, 'email')`.
- **Disable**: `api.deleteSavedSearch(search.id)`.
- **Sync**: `/my/profile`'s existing `handleSaved` callback (already fired
  after a successful profile PUT, regardless of where the toggle UI lives)
  additionally recomputes the query and calls `api.updateSavedSearch(search.id, { query })`
  when a profile-derived search exists.

## Risks / Trade-offs

- [Risk] A profile save now fires an extra network call (the sync PATCH)
  when the toggle is on. → Mitigation: fire-and-forget, best-effort — a
  failed sync is logged client-side only and does not block or roll back
  the profile save; the next successful save re-syncs.
- [Risk] `filtersFromProfile()`'s flattening is lossy by its own documented
  design (base vs. relocation location merge) — the auto-search is a
  convenience narrowing, not a precise expression of every profile nuance.
  Accepted; matches "Apply my profile"'s existing behavior exactly, so this
  is not a new limitation.

## Migration Plan

One additive migration: `saved_searches.derived_from_profile boolean NOT
NULL DEFAULT false` + the partial unique index. No backfill, no existing-row
rewrite, safe ahead of the code that reads it.
