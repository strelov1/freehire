// The overview every API reference opens with, applied to the namespace by docs.tsp.
// web/scripts/gen-api-docs.mjs renders it into /docs/api and docs/API.md.

namespace Freehire;

const ApiOverview = """
## Base URL

All endpoints are served under `https://freehire.me/api/v1`. The API is read-first and open: the job, search, facet, and company endpoints need no authentication and may be called cross-origin.

Authenticated endpoints accept either the browser session cookie (set by sign-in, same-origin) or a personal API key sent as a Bearer token — see the authentication model below. Keys are created in your account, at [freehire.me/my/integrations/api-keys](https://freehire.me/my/integrations/api-keys), not through this API.

## Response envelope

Single items are wrapped as `{ "data": ... }`. Collections add pagination metadata: `{ "data": [...], "meta": { "total", "limit", "offset" } }`. Errors are `{ "error": "message" }` with a matching HTTP status.

```json
{ "data": { "...": "single item" } }

{ "data": [ ... ], "meta": { "total": 4213, "limit": 20, "offset": 0 } }

{ "error": "job not found" }
```

## Pagination

List and search endpoints page with `limit` (default 20, max 100) and `offset` (default 0). `meta.total` reports the total matching the current filters, so you can compute the number of pages.

Pagination is bounded on **every** list endpoint: `offset + limit` may not exceed 10000 (`pagination too deep` → 400). This is deep-paging protection, not a cap on the reported total — use filters to narrow rather than paging that far.

The bound is a refusal rather than a clamp, deliberately: a clamped page would answer 200 carrying rows you did not ask for, and a client walking pages would loop over the same page forever without being able to tell. A 400 means "there is no such page", which is the truth.

## Errors

Errors use standard HTTP status codes: 400 (bad request / invalid value), 401 (missing or invalid credentials), 403 (authenticated but not allowed, e.g. a non-moderator), 404 (no such job, company, or owned resource), 429 (request limit exceeded), and 503 (service temporarily unavailable). Application errors use `{ "error": ... }`. nginx rate-limit rejections can return a non-JSON body; see API plans below.

## API plans

The public read endpoints answer without a key for personal and non-commercial use. Showing the postings in a product you offer to others needs an [API plan](https://freehire.me/jobs-api), under the [data licence](https://freehire.me/terms#data-licence).

Send a full-access key as `Authorization: Bearer <token>`, or use the OAuth access token of a signed-in MCP or CLI connection.

| Tier | Key | Account | Commercial use | Requests per 60 s | Results per page | Price |
| --- | --- | --- | --- | --- | --- | --- |
| Guest | none | none | no | 300 per IP for ordinary reads (`/jobs/search`, `/jobs/{slug}` and the rest), plus a separate 300 per IP for `/agent/jobs/search` | 100 | free |
| Free account | full-access key or OAuth (without one, the request is a guest's) | free sign-up | no | 300 per account, shared by ordinary reads and agent search | 100 | free |
| API plan | full-access key or OAuth | any freehire account | yes, under the data licence | 1,200 per account, shared by ordinary reads and agent search | 100 | $29 / month ($39 from 1 December 2026) |
| Data plan | full-access key or OAuth | any freehire account | yes, under the data licence | 6,000 per account on the Data endpoints (`/data/jobs/search`, `/data/jobs/{slug}`), counted apart; the API plan's 1,200 everywhere else | 100 | $99 / month ($129 from 1 December 2026) |

Pro and Ultra accounts (the AI job-search plans, which do not license the data) read at 800 and 1,000 requests per 60 seconds. Every tier gets the same filters, and no paid or free tier has a daily request cap. Prices are monthly, as of November 2026; [freehire.me/pricing](https://freehire.me/pricing) shows what is charged today.

Job reads do not carry the employer's own links: `url` is empty and `source_url` absent wherever `apply_url`, the freehire redirect to the same page, is set. The Data plan's endpoints serve both. The account shares one budget across its full-access API keys and OAuth connections, including public REST reads and agent search. A browser session cookie does not select the paid API rate.

The limit counts HTTP requests, not returned listings or unique vacancies. A page containing 100 listings costs one request. Re-fetching the same page or vacancy costs another request, including when the data has not changed. There is no separate cooldown for refreshing a vacancy.

Pagination returns 20 listings by default and up to 100 with `limit=100`. Requests carrying `offset` or `company_slug` also pass through separate per-IP nginx limits of 2 requests per second each, including paid requests. The `offset` limit allows a burst of 50 excess requests; the `company_slug` limit allows 30. A request carrying both parameters draws on both limits. For sustained pagination or company-filtered reads, keep the combined traffic from one IP within the applicable 2-request-per-second limit.

The gateway returns `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` (seconds until reset). A window starts with its first request. Exceeding a request limit returns HTTP 429. Gateway rejections include `Retry-After` in seconds; nginx rejections can omit these headers and return a non-JSON body. Pause for `Retry-After` when present; otherwise back off before retrying.

## Data freshness and closed jobs

Freshness in one line: each posting is re-read on its source's crawl schedule (no fixed interval is promised), and a posting a complete crawl stops listing is closed after 48 hours unseen by default (some sources wait longer, up to 35 days, when the source itself keeps postings up that long).

A posting is **open** until something closes it, and a closed posting carries `closed_at` and a `closed_reason`. Nothing is deleted. These are the closers, with the `closed_reason` each writes:

- The crawl no longer lists the posting: `unseen`. A crawl that ran incompletely, or whose board failed, closes nothing.
- A source that streams removals reports the posting taken down: `feed_removed`.
- Two consecutive liveness reads find the page expired, or the source answers that it is gone: `probe_expired`.
- A source with no close signal at all is closed by age (`expired`), and so is a posting whose application deadline has passed. The age rule is a guess, which is why the reason says so.
- A board whose crawl has failed for a long time (`board_unreachable`, `board_gone`) or has listed nothing for a long time (`feed_empty`), or a board that was retired (`board_retired`).
- An aggregator's copy of a posting whose employer's own board freehire already crawls closes in favour of that copy (`ats_covered`).
- A moderator (`moderated`), or the employer on a posting they published themselves (`employer_closed`).
- The employer deleting a posting they published themselves (`employer_deleted`). Unlike every other closer this is final: the posting leaves every list and `GET /jobs/{slug}` answers 404. The row is kept for the applications that reference it.

Postings closed before the reason was recorded carry an empty `closed_reason`. A closed posting can **reopen**: when its source lists it again, or a moderator reopens it, `closed_at` is cleared.

How often a posting is re-checked is set by each source's crawl schedule, and it differs by source; this API does not promise a figure. Search also withholds an open posting its source has not shown for 14 days, but only for sources where a missing listing is evidence: some sources have a longer window of their own, and self-closing, liveness-probed and manually added sources are excluded. An old `last_seen_at` alone does not prove a posting is out of search. Such a posting stays open and `GET /jobs/{slug}` serves it.

Closed postings never appear in `GET /jobs`, in search or in a company's open postings. `GET /jobs/{slug}` keeps serving them, with `closed_at` set, so a link you stored still resolves. A company's recent closed postings are at `GET /companies/{slug}/closed-jobs`. Interview prep also serves a closed public posting when the caller has a signed-in session and an application at the interview stage.

### Finding companies by domain

If you hold a list of company web domains, `GET /companies/by-domain?domain=stripe.com&domain=https://www.acme.io/careers` (repeatable or comma-joined, up to 100) returns `{"data": [{"domain", "slug", "name", "job_count", "website"}], "meta": {"unmatched": [...]}}`. Domains are compared by host (lowercase, no `www.`, port or trailing dot) against each company's site; every match is returned, including companies with `job_count` 0, and domains with no match are listed in `meta.unmatched`. Then read a company's postings with `GET /jobs/search?company_slug=<slug>` (repeatable), or subscribe a change webhook with `company_slugs` to have only their changes pushed.

### Keeping a mirror in sync

`GET /jobs/changes` is the feed of every opening, closing and reopening of a public, canonical posting. To keep a copy of the catalogue current:

1. Load the postings you want through the list and search endpoints, and note the time you started.
2. Call `GET /jobs/changes?since=<that time>` and follow `meta.next_cursor` as `cursor` until `meta.has_more` is false. Store the last `next_cursor`.
3. From then on, call `GET /jobs/changes?cursor=<stored cursor>` on your own schedule. A `closed` change means delist the posting; `opened` and `reopened` mean fetch it with `GET /jobs/{slug}` and list it.

The cursor is a position, not a time, so a walk never skips or repeats a change while new ones arrive. Each change carries a `seq`: for a given `public_slug`, apply a change only when its `seq` is greater than the last one you applied, and skip it otherwise. The feed can serve one posting's changes slightly out of order, and the first call with `since` can repeat a few changes from just before it; this rule makes both harmless. `since` can reach back 30 days at most, and the feed begins when it was introduced (`meta.available_since`). A change shows up in the feed shortly after it happens, not at the same instant: the feed trails the oldest transaction still open in the database, so a long-running one delays new entries until it ends. Nothing is lost, only late.

The feed keeps the full history and nothing is deleted from it, so a `cursor` you hold never expires. Only `since` is capped, at 30 days back, so a first call stays cheap: a mirror older than that rebuilds from search and then starts the feed with `since`.

A posting counts as listed while it is open, canonical (not a duplicate of another posting) and public. A posting that stops being listed is a `closed` change whose `closed_reason` is the close reason, or `duplicate` when it became a duplicate of another posting, or `private`; one that is listed again is `reopened`.

### Change webhooks

Instead of polling the feed you can have it pushed. `POST /me/change-webhooks` subscribes a URL (with an `api` or `api_data` plan); the answer carries the signing `secret`, **once**. A subscription can ask for only some companies (`company_slugs`) and some kinds of change (`kinds`), and receives changes made from about the time it was created (a change from the moment just before can arrive once; `seq` dedupes it). It reads the same feed as `GET /jobs/changes`, in the same order, and the same postings.

A delivery is a `POST` with a JSON body and these headers:

| Header | Value |
|---|---|
| `X-Freehire-Timestamp` | Unix seconds when it was signed. |
| `X-Freehire-Signature` | `v1=` plus the hex HMAC-SHA256, keyed by your secret, of the timestamp, a `.`, and the raw body. |
| `X-Freehire-Delivery` | An id, the same for a retry of the same batch. |
| `X-Freehire-Event` | `job.changes`, or `webhook.test` for a test. |

```json
{"id": "7-4821-90210", "type": "job.changes", "changes": [
  {"public_slug": "senior-go-engineer-acme-1a2b", "company_slug": "acme", "seq": 98765, "kind": "closed", "at": "2026-06-20T04:10:00Z", "closed_reason": "unseen"}
]}
```

`changes` is a batch of up to 100, oldest first, each shaped like a feed change plus `company_slug`. Apply a change only when its `seq` is greater than the last one you applied for that `public_slug` (this also drops a repeat). `opened` and `reopened` mean fetch the posting with `GET /jobs/{slug}`; `closed` means delist it.

Verify before you trust a body: recompute the HMAC over the timestamp header, `.` and the **raw** body bytes (not re-serialised JSON), compare in constant time, and refuse a timestamp more than 5 minutes from your clock. In Go:

```go
func verify(secret, ts, sig string, body []byte) bool {
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || math.Abs(time.Since(time.Unix(n, 0)).Seconds()) > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s.", ts)
	mac.Write(body)
	return hmac.Equal([]byte("v1="+hex.EncodeToString(mac.Sum(nil))), []byte(sig))
}
```

Answer any `2xx` to accept a batch. Anything else, a timeout (10 seconds) or a redirect counts as a failure: the same batch is sent again after 30 seconds, doubling up to an hour. Delivery is at least once, so expect repeats and dedupe on `seq`. After 25 failures in a row, or at once on a `410 Gone`, the subscription is disabled (`disabled_at` and `disabled_reason` in the list); delete it and create it again to resume, from the changes made then. Plan for the feed instead if you need what happened meanwhile. While your plan has lapsed, deliveries pause and pick up where they stopped once it is renewed.

## Authentication model

Browser clients authenticate with an `HttpOnly` session cookie set on sign-in (same-origin; the SPA cannot read it). Non-browser clients use a personal API key as `Authorization: Bearer <token>`.

Endpoints marked “Session or API key” accept either; endpoints marked “Session only” (saved searches, subscriptions) accept only the cookie, so a leaked key cannot manage credentials. “Moderator” endpoints additionally require the moderator role.

## What is not here

This reference covers every endpoint you can call. A handful are deliberately left out because calling them directly is meaningless: the Gmail and calendar consent redirects (`/me/gmail/connect`, `/me/calendar/connect`, and their callbacks), which only a browser can complete; the Telegram bot webhook; the browser-tool websocket relay; the sitemap-cursor helpers behind `/sitemap.xml`; and the `/og/*.png` social-preview cards, which render an image rather than answer with JSON.

Managing API keys (`POST`, `GET` and `DELETE` on `/me/api-keys`) is not documented here, because no API client can call it. Those endpoints accept only the browser session cookie, and additionally require a freshly proven identity — a password re-entry or a sign-in-provider check — that a script has no way to supply. Create and revoke keys in your account instead, at [freehire.me/my/integrations/api-keys](https://freehire.me/my/integrations/api-keys); the plaintext token is shown exactly once, at creation, so store it then.

The `/jobs/{slug}/fit` endpoints are pre-rename aliases of `/jobs/{slug}/match-analysis` and hit the same handlers. They still work, so existing clients do not break — use the match-analysis paths in new code.

Most of this API is also a CLI. If you are writing an agent rather than an integration, `freehire` covers the same surface with less ceremony — search, tracking, the inbox and CV tailoring — over one API key.
""";
