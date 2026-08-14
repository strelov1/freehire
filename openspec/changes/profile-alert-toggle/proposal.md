## Why

A candidate who wants "tell me about jobs matching my profile" today has to
manually open the filters panel, hit "Apply my profile," save the result as a
saved search, then subscribe a channel — and nothing keeps that search in
sync if the profile changes afterward.

## What Changes

- A new toggle on `/my/profile`: "Notify me about jobs matching my profile."
  Turning it on creates a saved search from the current profile (reusing the
  existing client-side `filtersFromProfile()` derivation — the same one
  "Apply my profile" already calls) and subscribes it on the account's
  default notification channel. Every subsequent profile save refreshes that
  search's filters. Turning it off deletes the search (and its subscription
  with it).
- `saved_searches` gains `derived_from_profile boolean`, with a partial
  unique index enforcing at most one per user — this is what lets the UI
  and the create endpoint find/guard "the" profile search without a
  client-supplied id.
- `POST /me/searches` gains an optional `derived_from_profile` flag.
- No new matching engine: the toggle is a thin convenience layer over
  saved-search + subscription primitives `internal/notify` already has,
  including the digest-frequency/quiet-hours controls just shipped.

## Capabilities

### New Capabilities
- `profile-alert-toggle`: the `/my/profile` toggle, its create/sync/delete
  behavior, and the default-channel rule.

### Modified Capabilities
- `saved-searches`: "Save a filter set" gains the optional
  `derived_from_profile` flag and the per-user at-most-one invariant it
  enforces.

## Impact

- **Migration**: `saved_searches.derived_from_profile boolean NOT NULL
  DEFAULT false` + partial unique index `(user_id) WHERE
  derived_from_profile`.
- **Go**: `internal/savedsearch` — `Create` accepts the flag; a new
  `ErrProfileSearchExists` sentinel (409) guards the invariant.
  `internal/handler/me_searches.go` — request/response shape gains the
  field.
- **SPA**: a new toggle component on `/my/profile`'s Settings tab; a hook
  into `ProfileForm`'s save-success path to sync the profile search's query
  when the toggle is on.
