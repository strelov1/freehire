## Context

The repo already ships the whole "get new jobs matching a filter as a Telegram
digest" pipeline: a `saved_searches` row stores a canonical query string, a
`subscriptions` row (channel `telegram`) rides on it, and `cmd/notify` matches the
query against Meilisearch and delivers digests titled by the saved-search name.
Crucially, `company_slug` is already a first-class filterable search attribute
(`internal/search/client.go` `FilterableAttributes`, mapped in
`StringFacets`), so a filter of exactly one company is already expressible and
matchable.

On the web side, `CompanyView.svelte` renders the company header + a company-
scoped `JobsView`. Two client stores already encapsulate the follow primitives:
`savedSearches` (`create`/`update`/`remove`/`items`) and `notifications`
(`telegram` status, `forSavedSearch`, `subscribe`, `unsubscribe`, `link`,
`refreshTelegram`). `SavedSearches.svelte` already implements the Telegram
link-then-subscribe UX for the active filter.

This change adds a company-page button that drives those same stores. It is
frontend-only. The web app has no test runner (only `svelte-check` + lint, whose
baseline is already red), so verification is `svelte-check` plus a manual pass.

## Goals / Non-Goals

**Goals:**
- One-click follow/unfollow of a company's new postings from its page.
- Reuse existing saved-search + subscription primitives with zero backend change.
- Never destroy a user's manually-saved filter for the same company.
- Match the established Telegram connect UX (parity with `SavedSearches.svelte`).

**Non-Goals:**
- No in-app "followed companies" feed or in-app notifications (delivery stays
  Telegram-only, as today).
- No new backend tables, endpoints, or `cmd/notify` logic.
- No refactor of `SavedSearches.svelte` (shared-logic extraction is a possible
  future step, not this change).
- No email/other channels.

## Decisions

### D1: Compose existing primitives instead of a new "follow" entity
A company follow is modelled as `saved_search(query="company_slug=<slug>")` +
`subscription(telegram)`. Alternative: a dedicated `followed_companies` table +
endpoints + notify path. Rejected — it duplicates the subscription machinery and
contradicts the MVP "compose, don't multiply entities" principle. The query is
built canonically (`canonicalQuery` over `company_slug=<slug>`) so it matches the
same way the filters panel and `cmd/notify` produce/consume it.

### D2: New focused component, stores unchanged
Add `CompanyFollowButton.svelte`, mounted in the `CompanyView` header. It reads
`savedSearches.items` + `notifications` to derive follow state and calls the
existing store methods. Alternative: inline the logic into `CompanyView`.
Rejected — keeps `CompanyView` a thin composition and isolates the follow
state-machine in one testable-by-inspection unit.

### D3: Reuse-or-create on follow; name-guarded delete on unfollow
Follow finds a saved search whose canonical query equals the company query and
reuses it; otherwise creates one named exactly the company name. Unfollow always
deletes the subscription, and deletes the saved search **only if its name equals
the company name** (i.e. we generated it). This protects a user-owned filter that
happens to target the same company but is named differently. Alternative:
always-delete (risks nuking a manual filter) or never-delete (clutters "My
filters"). The name-guard is the correct middle: clean toggle for our own rows,
safe for the user's.

### D4: Precondition handling mirrors SavedSearches
- Signed-out → `openAuthDialog()`, no write attempted.
- `telegram.enabled === false` (bot unconfigured server-side) → render nothing.
- Signed-in, Telegram not linked → open deep link (`notifications.link()`) and
  show an "I've connected" re-check (`notifications.refreshTelegram()`), exactly
  as `SavedSearches.svelte` does, before subscribing.
Stores are loaded via an `$effect` gated on `isAuthenticated()`, matching the
existing SSR-safe pattern (`ensureLoaded` is a browser-only no-op on the server).

## Risks / Trade-offs

- **Name-guard heuristic** → A user who manually named a filter exactly the
  company name and later unfollows would have that filter deleted. Rare and
  bounded; mitigated by first reusing any matching saved search on follow, so the
  common paths don't create duplicates.
- **Name collision on create** (user already has a differently-queried filter
  named exactly the company name → unique `(user, name)` 409) → surface a
  friendly error rather than silently retrying with a mangled name; acceptable for
  an edge case.
- **No automated test** → Web has no runner; mitigated by `svelte-check` and a
  manual pass, consistent with all existing web work in this repo.
- **Telegram-only delivery** → "Subscribe" implies alerts the user only gets in
  Telegram; the connect flow and hidden-when-disabled behavior make the dependency
  explicit rather than surprising.

## Migration Plan

Pure additive frontend change; no schema or API migration. Deploy is a normal web
build. Rollback = revert the component + the `CompanyView` mount. No data written
that isn't already valid saved-search/subscription rows.

## Open Questions

None outstanding — the auth/Telegram-linking behavior, unfollow semantics, and
button label were resolved during brainstorming.
