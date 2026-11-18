# Notifications: matching saved searches

Part of the [notifications](../notifications.md) conventions, the bullets of its "Always true" list about how `notify` matches saved searches to new jobs: one search per distinct query, only when a digest could go out, and the per-subscriber skill-match and avoid-skills filters applied after it.

- **Matching is O(distinct queries), not O(subscribers).** `notify.Runner.Run` groups
  subscriptions sharing a saved-search query so the search index is hit once regardless of
  how many people subscribed to it. A per-subscription loop would multiply index load by
  subscriber count.
- **A query is searched only when its digest could go out** (`notify.searchDue`). Grouping
  buys little in practice — measured 2026-11-05, 2,451 active subscriptions carried 2,332
  distinct queries — so the real cost was the cadence: every five-minute pass ran all
  ~2,100 of them while an hourly digest can use one search an hour, and Meilisearch was
  the busiest process on the host (~4 cores, ~530 MB/s read). A person's subscriptions
  (user and channel; a webhook alone) are searched together: `instant` every pass;
  `interval` once the interval since the last message has elapsed, outside quiet hours,
  and then once per interval while nothing is found; `daily` when `DigestDue` says the
  day's digest is due, hourly while it stays due. The result is that a posting is found
  no later than one interval after it appears and sent in that same pass — the latency
  the interval already promised. `subscriptions.last_searched_at` (migration 0239) holds
  when a subscription was last searched, stamped with the pass's START, and a search
  after a gap **pages back** to it (less `matchLag`, an hour, for the drain's indexing
  delay; at most `maxMatchPages`), so the gap loses nothing a five-minute scan would
  have recorded. A never-searched subscription is always due and reads one page. The
  run log's `skipped_queries` and `searches` (index requests, counting pages) are what
  measure it. **Carry-over sends are the one seam**: a message released earlier (a
  failed send, a relinked chat) can go out on a pass that did not search, and what was
  indexed since the last search then waits for the next due pass.
- **A saved search's minimum skill match is applied per subscriber, after the search.**
  `min_skill_match` is a web filter the search never reads: the percent depends on the reader, so
  `Runner.match` computes it per (hit, subscription) pair against the subscriber's live
  `user_profiles.skills`, with the same formula as the job list (`computeClientMatch`). It can
  only thin the pages the search read: on a broad query, jobs clearing the threshold but beyond
  those pages are not reached. A query carrying only this param does not narrow the catalogue, so
  a subscription to it is refused like any other unfiltered search (`search.Narrows`).
- **Avoid-skills is enforced as a per-subscriber post-filter, not folded into the shared search.**
  `Runner.match` batch-fetches every active subscriber's live `user_profiles.excluded_skills` once
  per pass (`Store.ListUserProfileSkillSets`) and `matchQuery` skips a `(hit, subscription)`
  pair whose job carries a skill that subscriber currently avoids — evaluated against the *live*
  preference, not whatever `skills_exclude` (if any) got frozen into the saved search's own query
  string at creation time. Do not move this into the Meilisearch `Filter`: that would make the
  filter subscriber-specific and defeat the canonical-query grouping above, turning matching back
  into O(subscribers).
