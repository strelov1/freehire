## Context

`internal/notify.Runner.match` (`internal/notify/match.go`) groups active subscriptions by their
saved search's canonical query string and runs each distinct query against the search index exactly
once, then fans the hits out to every subscription sharing that query
(`internal/notify/match.go:22-94`). `docs/agents/notifications.md` calls this out as a load-bearing
property: *"Matching is O(distinct queries), not O(subscribers). A per-subscription loop would
multiply index load by subscriber count."* Any fix must add per-subscriber avoid-skills enforcement
without turning distinct-query grouping back into a per-subscriber search.

`user_profiles.excluded_skills` (`internal/userprofile`) already stores each account's live avoid
list, normalized (lowercased/trimmed/deduped) on save (`internal/userprofile/userprofile.go`). Job
skills are served from the same canonical, dict-only vocabulary (`internal/skilltag`), and
`search.JobDocument` embeds `jobview.Job`, which already carries `Skills []string`
(`internal/jobview/card.go:27`) — so a hit's skills and a profile's excluded skills are directly
comparable strings, no fuzzy matching needed. See proposal.md for the motivating bug.

## Goals / Non-Goals

**Goals:**
- Exclude jobs carrying an avoided skill from a subscription's matches, evaluated against the
  subscriber's *current* `excluded_skills` at match time — self-healing for subscriptions created
  before an avoid-list edit.
- Preserve the O(distinct queries) search cost: no additional Meilisearch call per subscriber.

**Non-Goals:**
- Changing `internal/reminder` or `internal/nudge` (see proposal.md — out of scope).
- Changing the skill-tag dictionary or job skill derivation.
- Changing what "Apply my profile" seeds into a saved search's query string — that UI convenience
  keeps working as before; this change makes it unnecessary for avoid-skills specifically.

## Decisions

**Post-filter in Go after the single shared search, not a per-subscriber Meili filter.**
Folding `excluded_skills` into the Meili `Filter` would make the filter subscriber-specific, which
defeats the canonical-query grouping `match` depends on (`docs/agents/notifications.md`'s stated
invariant). Instead, `matchQuery`'s existing `for _, hit := range res.Hits { for _, s := range subs
{...} }` loop (already O(hits × subs) to test each subscription's `start_at`) gains one more
per-pair check: skip a `(hit, subscription)` pair when `hit.Skills` intersects that subscription's
subscriber's excluded skills. No new asymptotic cost, no new search call.

**Batch-fetch excluded skills once per `match` pass, keyed by user id, not once per query group.**
The same subscriber can appear in several query groups (multiple saved searches). Fetching once per
pass over the union of every active subscription's `user_id` — one query — is fewer round trips than
fetching per group or per subscription. The new `ListUserProfilesExcludedSkills` query
(`internal/db/queries/user_profiles.sql`) is added directly to `internal/notify`'s existing `Store`
interface, alongside `ListActiveSubscriptions`, `GetJobsForDigest`, etc. — every other `Store` method is
already a thin pass-through to a generated `*db.Queries` method with no domain-package indirection, so
`internal/userprofile` is deliberately NOT introduced as a dependency of `internal/notify`: it would
duplicate the DB wiring already passed into `notify.New` for no benefit, since nothing beyond raw
persistence is needed here. `Runner.match` calls the new `Store` method once after
`ListActiveSubscriptions`, builds a `map[int64][]string`, and passes it down into `matchQuery`. A user
id absent from the map (no profile row yet) is treated as an empty exclude set — the same default as
everywhere else `excluded_skills` is read.

**Exact case-insensitive string-set membership, no partial/fuzzy matching.** Both sides already come
from the same canonical dict-only vocabulary (`internal/skilltag`), normalized the same way
(`internal/userprofile` lowercases/trims on save; job skills are stored as canonical dictionary
strings). A direct set intersection is correct and matches how `skills_exclude` already behaves in
the search filter path (`internal/search/query_filter.go`).

## Risks / Trade-offs

- [Risk] A job whose relevant skill is mentioned in the description under an alias not yet in the
  skill-tag dictionary is not tagged on the job, so it cannot be excluded by this check. →
  Accepted pre-existing gap (see proposal.md "Impact"/"Out of scope"); not something a notification-
  layer fix can address. Worth flagging if it recurs, as a dictionary-coverage gap in
  `internal/skilltag`, not a notify bug.
- [Risk] Matches already recorded in `subscription_matches` before this change (or before a user's
  most recent avoid-list edit) are not retroactively purged — `MATCH` only gates going forward, and
  `DELIVER` sends whatever is already claimed in the ledger. → No backfill needed: this is a one-time
  transitional edge (the next `MATCH` pass after a user edits their avoid list stops adding new
  matching pairs; any pair already recorded before that edit was, at the time, a legitimate match).
  Not worth a special-case purge for a preference change.

## Migration Plan

No schema or migration changes. Deploy the updated `internal/notify` binary normally; the next
`cmd/notify` run picks up the new behavior. No rollback complexity beyond reverting the code — the
match ledger's idempotency (`RecordSubscriptionMatches`) is unaffected either way.
