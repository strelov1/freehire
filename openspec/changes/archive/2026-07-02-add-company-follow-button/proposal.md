## Why

A visitor who cares about one company has no way to be told when that company
posts a new job — they must remember to re-open the company page and re-scan it.
The backend already treats `company_slug` as a first-class search filter, and we
already deliver "new jobs matching a saved filter" as Telegram digests. A
LinkedIn-style "Subscribe to updates" button on the company page turns that
latent capability into a one-click action.

## What Changes

- Add a **Subscribe to updates** toggle button to the company page header
  (`CompanyView`). Signed-in users with Telegram available can follow a company
  and receive its new postings as Telegram digests; the button reflects the
  current follow state (`Subscribe to updates` ⇄ `Subscribed`).
- Following a company **reuses the existing saved-search + filter-subscription
  primitives**: find-or-create a saved search with `query = "company_slug=<slug>"`
  (named after the company) and create a Telegram subscription on it. No new
  backend tables, endpoints, or worker logic.
- Unfollowing is a **clean toggle**: it deletes the Telegram subscription and,
  only when the saved search was the one we generated (its name matches the
  company name), the saved search too — so a user's manually-saved filter for the
  same company is never destroyed.
- Frontend-only. The button is hidden when the Telegram bot is not configured
  server-side; signed-out users are routed to the auth dialog; an unlinked
  Telegram walks the same deep-link connect flow as the "My filters" panel.

## Capabilities

### New Capabilities
- `company-follow`: the company page's "Subscribe to updates" action — how a user
  follows/unfollows a company for new-job Telegram alerts, expressed on top of the
  saved-search and filter-subscription capabilities.

### Modified Capabilities
<!-- None: no existing spec's requirements change. company-follow composes the
     saved-searches and filter-subscriptions capabilities without altering them. -->

## Impact

- **Code (web SPA only)**: new `web/src/lib/components/CompanyFollowButton.svelte`;
  `web/src/lib/components/CompanyView.svelte` mounts it in the header. Reuses the
  existing `savedSearches` and `notifications` stores unchanged.
- **Backend**: none. `company_slug` is already a filterable search attribute and
  `cmd/notify` already matches saved-search queries.
- **Verification**: no web test runner in the repo — verify via `svelte-check`
  (and a manual/visual pass), consistent with existing web practice.
