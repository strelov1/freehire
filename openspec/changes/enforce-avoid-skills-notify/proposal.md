## Why

A user's "avoid skills" preference (`user_profiles.excluded_skills`) is never enforced when saved-search
subscriptions are matched against new jobs. It is applied only as a one-time, manual UI convenience: clicking
"Apply my profile" seeds `skills_exclude=…` into the search URL, which gets frozen into a saved search's
`query` string at creation time. `internal/notify/match.go`'s `matchQuery` only ever parses that frozen query
string — it never reads the account's *current* `excluded_skills` at match time. So a subscription created
without clicking "Apply my profile", or created before a skill was added to the avoid list, never excludes it,
and later editing the avoid list never retroactively affects any existing subscription. Users are receiving
job notifications containing skills they explicitly asked to avoid.

## What Changes

- `internal/notify`'s subscription-matching path fetches the subscriber's live `excluded_skills` from
  `internal/userprofile` and applies it as an additional exclude filter when matching jobs, independent of
  whatever `skills_exclude` (if any) is frozen into the saved search's own query string.
- This applies at match time, so it self-heals: a subscription created before the user set an avoid list, or
  before a later edit to that list, honors the *current* list on the very next matching pass — no need to
  recreate or re-save the subscription.
- Out of scope: `internal/reminder` (saved-job reminders) and `internal/nudge` (lifecycle nudges) — both act
  on jobs the user explicitly saved or applied to, not on a search query, so an avoid-skills exclude filter
  does not apply to them.
- Out of scope: changing what "Apply my profile" does in the frontend, or the skill-tagging dictionary
  (`internal/skilltag`) itself — a job whose relevant skill isn't tagged (untagged alias) is a pre-existing
  dictionary-coverage gap, not something this change can fix.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `filter-subscriptions`: the "Windowed filter matching" requirement gains a live avoid-skills exclusion —
  matching a subscription's canonical query against the search index MUST additionally exclude jobs carrying
  any skill in the subscriber's current `excluded_skills`, evaluated at match time rather than frozen into the
  saved search's query.

## Impact

- `internal/db/queries/user_profiles.sql` — a new batch query fetching `excluded_skills` for a set of
  user ids in one round trip.
- `internal/notify` (`Store` interface, `Runner.match`, `matchQuery`) — fetch every active subscriber's
  `excluded_skills` once per pass and post-filter each `(hit, subscription)` pair in Go, rather than
  folding it into the Meili filter (see design.md for why: a per-subscriber filter would defeat the
  canonical-query grouping the matching cost model depends on).
- No changes to `internal/userprofile` or `internal/search` — the new query is added directly to
  `internal/notify`'s own `Store` interface, consistent with how every other `Store` method there is
  already a thin pass-through to a generated `*db.Queries` method.
- No schema or migration changes — `user_profiles.excluded_skills` already exists and is already populated by
  the existing profile UI.
