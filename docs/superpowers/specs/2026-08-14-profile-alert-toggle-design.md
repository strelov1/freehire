# Profile alert toggle: one switch, no manual filter-building

## Problem

A candidate who wants "tell me about jobs matching my profile" today has to
manually open the filters panel, hit "Apply my profile" (which stages the
filters), save it as a saved search, then subscribe a channel. Nothing keeps
that search in sync if the profile changes afterward.

## Goal

A single toggle — "Notify me about jobs matching my profile" — on
`/my/profile`. Turning it on creates a saved search from the current profile
and subscribes it (default channel); every subsequent profile save keeps that
search's filters current; turning it off removes both. No new matching
engine — the toggle is a convenience wrapper over the saved-search +
subscription primitives `internal/notify` (and everything just shipped —
frequency, quiet hours, channels) already deliver.

## Storage

- `saved_searches` gains `derived_from_profile boolean NOT NULL DEFAULT
  false`, plus a partial unique index `(user_id) WHERE derived_from_profile`
  so at most one such row exists per user — the server-owned invariant that
  makes "find the profile search" unambiguous without trusting a
  client-supplied id.

## Decisions

- **The filter-derivation logic is NOT duplicated server-side.** `web/src/lib/facetModel.ts`'s
  `filtersFromProfile()` (the exact function "Apply my profile" already
  calls) is the only place that turns a profile into a query string. The
  toggle reuses it client-side and POSTs/PATCHes the resulting query through
  the existing saved-search API — porting that derivation to Go would be a
  second copy of real business logic (location-preference flattening,
  skill-include/exclude overlap rules) that could drift from the one users
  already see when they click "Apply my profile" manually.
- **Sync-on-save is client-driven, not a backend hook.** There is exactly one
  way to edit a profile — the SPA's `ProfileForm` — so after every successful
  save, if the toggle is on, the client recomputes the query and calls the
  existing `PATCH /me/searches/:id`. No new backend trigger, no risk of the
  derivation logic disagreeing with itself between two implementations.
- **`derived_from_profile` is create-time only, immutable.** `CreateSavedSearch`
  accepts an optional `derived_from_profile: true`; `UpdateSavedSearch` never
  changes it. Server-side, a create with the flag set 409s
  (`ErrProfileSearchExists`) if one already exists for the user — the client
  always checks its already-loaded `savedSearches` store first, so this is a
  race guard, not the primary control flow.
- **Turning off deletes the row** (existing `DELETE /me/searches/:id`),
  rather than leaving an orphaned unsubscribed search behind. Re-enabling
  later creates a fresh one. If the user manually deletes their profile
  search from `/my/notifications/searches`, the profile-page toggle reads
  that as "off" (it just checks whether a `derived_from_profile` row exists)
  — no special-cased recovery needed.
- **Default channel on creation**: the account's current
  `notification_settings.channels` (falling back to `['email']` if empty/never
  configured) — the same default `ScheduleOnSave` already uses for reminders,
  so the toggle doesn't need its own channel-picker step; the user can still
  fine-tune per-channel from the normal `AlertChannels` control once the
  search exists (it is an ordinary saved search from every other API's point
  of view).
- **Everything downstream is free.** Once the row exists, `internal/notify`'s
  existing MATCH/DELIVER, the digest-frequency/quiet-hours gating just
  shipped, and the `/my/notifications/searches` list all treat it like any
  other saved search — no engine changes.

## API

- `POST /me/searches` gains an optional `derived_from_profile: boolean`
  field (default false, unchanged for every existing caller).
- No other new endpoints. Enable = `POST /me/searches` (with the flag) +
  `POST /me/subscriptions`. Disable = `DELETE /me/searches/:id`. Sync =
  `PATCH /me/searches/:id`.

## UI

- A toggle on `/my/profile` (Settings tab, near `AccountTimezone`, matching
  its autosave-card style): "Notify me about jobs matching my profile."
  State is derived from the already-loaded `savedSearches` store (does a
  `derived_from_profile` row exist for this account) — no separate flag to
  fetch.
- On toggle-on: compute `filtersFromProfile(profile)`, create the search
  (name e.g. "My profile matches"), subscribe the default channel.
- On toggle-off: delete that search.
- On every successful profile save: if a `derived_from_profile` search
  exists, recompute and `PATCH` its query.

## Out of scope

- A true skill-coverage-threshold matching engine (only notify above e.g.
  70% fit) — this ships the "auto-generated saved search" version only, per
  explicit product decision.
- A channel-picker step in the toggle itself — defaults to the account's
  existing notification channels; edit from the search's own row afterward.
