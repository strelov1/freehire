import "@typespec/openapi";

using TypeSpec.OpenAPI;

// The reader-facing half of the contract: the overview every API reference opens with and
// the description of each tag. web/scripts/gen-api-docs.mjs renders the docs from
// openapi/api.openapi.json, so what is written here is what /docs/api and docs/API.md say.

@doc("""
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

The API plan allows 1,200 requests per 60-second window. The Data plan allows 6,000. Neither paid API plan has a daily request cap. The account shares one budget across its full-access API keys and OAuth connections, including public REST reads and agent search. A browser session cookie does not select the paid API rate.

The limit counts HTTP requests, not returned listings or unique vacancies. A page containing 100 listings costs one request. Re-fetching the same page or vacancy costs another request, including when the data has not changed. There is no separate cooldown for refreshing a vacancy.

Pagination returns 20 listings by default and up to 100 with `limit=100`. Requests carrying `offset` or `company_slug` also pass through separate per-IP nginx limits of 2 requests per second each, including paid requests. The `offset` limit allows a burst of 50 excess requests; the `company_slug` limit allows 30. A request carrying both parameters draws on both limits. For sustained pagination or company-filtered reads, keep the combined traffic from one IP within the applicable 2-request-per-second limit.

The gateway returns `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` (seconds until reset). A window starts with its first request. Exceeding a request limit returns HTTP 429. Gateway rejections include `Retry-After` in seconds; nginx rejections can omit these headers and return a non-JSON body. Pause for `Retry-After` when present; otherwise back off before retrying.

## Authentication model

Browser clients authenticate with an `HttpOnly` session cookie set on sign-in (same-origin; the SPA cannot read it). Non-browser clients use a personal API key as `Authorization: Bearer <token>`.

Endpoints marked “Session or API key” accept either; endpoints marked “Session only” (saved searches, subscriptions) accept only the cookie, so a leaked key cannot manage credentials. “Moderator” endpoints additionally require the moderator role.

## What is not here

This reference covers every endpoint you can call. A handful are deliberately left out because calling them directly is meaningless: the Gmail and calendar consent redirects (`/me/gmail/connect`, `/me/calendar/connect`, and their callbacks), which only a browser can complete; the Telegram bot webhook; the browser-tool websocket relay; the sitemap-cursor helpers behind `/sitemap.xml`; and the `/og/*.png` social-preview cards, which render an image rather than answer with JSON.

Managing API keys (`POST`, `GET` and `DELETE` on `/me/api-keys`) is not documented here, because no API client can call it. Those endpoints accept only the browser session cookie, and additionally require a freshly proven identity — a password re-entry or a sign-in-provider check — that a script has no way to supply. Create and revoke keys in your account instead, at [freehire.me/my/integrations/api-keys](https://freehire.me/my/integrations/api-keys); the plaintext token is shown exactly once, at creation, so store it then.

The `/jobs/{slug}/fit` endpoints are pre-rename aliases of `/jobs/{slug}/match-analysis` and hit the same handlers. They still work, so existing clients do not break — use the match-analysis paths in new code.

Most of this API is also a CLI. If you are writing an agent rather than an integration, `freehire` covers the same surface with less ceremony — search, tracking, the inbox and CV tailoring — over one API key.
""")

// Listed last-first: decorators apply bottom-up, and the reference shows the tags in
// the order they are applied.
@tagMetadata("Watchlist", #{})
@tagMetadata("Tailoring", #{})
@tagMetadata("Support", #{})
@tagMetadata("Promo", #{})
@tagMetadata("Plans", #{})
@tagMetadata("Plan", #{})
@tagMetadata("Moderation", #{})
@tagMetadata("Mentorship", #{})
@tagMetadata("Insights", #{})
@tagMetadata("Employer", #{})
@tagMetadata("Email preferences", #{})
@tagMetadata("DirectApply", #{})
@tagMetadata("Dictionary", #{})
@tagMetadata("Conversations", #{})
@tagMetadata("Company relations", #{})
@tagMetadata("Buddies", #{})
@tagMetadata("Billing", #{})
@tagMetadata("Autofill", #{})
@tagMetadata("Auto-apply", #{})
@tagMetadata("Answer bank", #{})
@tagMetadata("Mail", #{ description: "Your job mail, and the link between a message and the application it belongs to. Mail arrives three ways: a connected Gmail account, the hosted freehire address, or a batch your own client pushed. Only the first two are classified by freehire — pushed mail is yours to triage, which is what makes that tier free. Everything here takes a full-scope API key, so a harness can drive the whole surface; the Gmail consent redirect is the one exception and stays browser-only." })
@tagMetadata("Experience", #{ description: "The durable record a CV is built from: employments, and the evidence atoms under them. Every atom carries who asserted it — you, or a model that inferred it — and only your own assertions may be written into a CV. Reading accepts an API key; editing is session-only." })
@tagMetadata("CV", #{ description: "Your CVs, and the tailoring flow that reframes one toward a single vacancy. Authoring is browser-owned — create, update and delete are session-only — while reading a CV, patching it, and rendering the PDF also accept an API key, because that is the half a tailoring agent drives. Every CV is owner-scoped: an id you do not own is a 404, not a 403." })
@tagMetadata("Talent Network", #{ description: "A public, anonymised catalogue of candidates who opted into being found. Membership is a single toggle on `users`, distinct from `user_profiles`; a member is addressed by a minted handle, never by their account id or username. Every string a public response carries is a value a dictionary resolved, a number or a date — never text copied from a CV — so no name, employer, institution or free-text field appears anywhere in it." })
@tagMetadata("Referrals", #{ description: "Two sides of the same marketplace: an employee offers to refer into the company they work at (proof required, moderated), and a candidate asks one of that company's approved referrers for an intro. A candidate never sees who the referrer is, and a referrer sees the candidate's CV only through a short-lived view. Moderation of offers is moderator-gated." })
@tagMetadata("Community", #{ description: "The lighter per-user surfaces: a vote on a job or company, the account-level notification rule (gates the saved-job apply reminder and both lifecycle nudges), and the public discussion threads. Votes and notification settings take a key; posting to a thread is browser-owned." })
@tagMetadata("Contributions", #{ description: "Turning a URL in the wild into something freehire knows about. One intake sequence serves every surface — the website, the bot, the extension and the CLI — deliberately, so a pasted link gets the same answer everywhere." })
@tagMetadata("Alerts", #{ description: "Browser conveniences, session-only. A saved search stores a canonical filter query string; a subscription turns one into a recurring digest (Telegram, email, or your own webhook). Each operation is owner-scoped — a non-owned id is a 404." })
@tagMetadata("Stats", #{ description: "Aggregate market intelligence, all public and all unauthenticated. Every figure is served from a precomputed rollup rather than counted live, and nothing here exposes a record-level field or a user identifier — these are counts over the catalogue, not a way to read it." })
@tagMetadata("Account", #{})
@tagMetadata("Résumé", #{ description: "Your career profile and stored CV, session-only (a browser feature, like saved searches). The profile is a singleton keyed by your session — no id in the path. The verdict and ATS report are read-only sub-resources computed from the profile and CV; résumé storage degrades to `501` when object storage is unconfigured." })
@tagMetadata("Feedback", #{ description: "Votes on jobs and companies, and ghost reports: one person states they applied to a posting and were never answered, feeding the ghost-signal evidence used elsewhere in the API (see the `reality` field on a job). Nothing here reaches a moderator and nothing here closes the job directly." })
@tagMetadata("Reports", #{ description: "Any signed-in user can flag a problem with a live vacancy. Review actions are moderator-only; resolving may soft-close the reported job." })
@tagMetadata("Submissions", #{ description: "Any signed-in user can submit a vacancy for moderation and read their own queue. The review actions are moderator-only; approval mints a live job." })
@tagMetadata("Assistant", #{ description: "The same agent the web app and browser extension chat with, over HTTP: create a conversation, then post messages to it. All routes accept the session cookie or an API key (`autopilot` is cookie-only — see below) and act on a session you own; someone else’s session id answers `404`, same as one that never existed. Turns run one at a time per session — a message sent while one is already running queues rather than running alongside it. `preset` selects which conversation this is: `chat` (default), `profile` (the experience interviewer), `browse` (a browsing session held from the extension’s side panel), `interview` (a mock-interview rehearsal), `debrief` (after a real one) or `apply` (the conversation about one automatic application) — the last three must name the vacancy via `?job=<slug>`, an application you already hold. An application holds one `apply` conversation: asking again returns it with `200` instead of creating a second. A `tailor` session cannot be created here; it is minted by the CV tailoring bootstrap, which knows the CV and vacancy to bind it to." })
@tagMetadata("Tracking", #{ description: "Per-user tracking, addressed by the job slug. Writes are idempotent. The response is the interaction record for that job." })
@tagMetadata("Auth", #{ description: "Register/login set the session cookie and return the user. Logout clears it. `me` resolves the caller (cookie or API key). OAuth sign-in is a redirect flow." })
@tagMetadata("Company feedback", #{ description: "Signed-in users leave a 1–5 star rating plus category and text about a company, shown under their site-wide pseudonymous persona (never a user id). Reads are public; writes are cookie-only. A reader can report a specific review, and a moderator can hide it." })
@tagMetadata("Search", #{ description: "Full-text job search with the shared filter vocabulary, facet counts, similar jobs and the reference data behind search." })
@tagMetadata("Companies", #{ description: "Public reads. A company detail also returns a page of its open jobs." })
@tagMetadata("Fit", #{ description: "Personalized signals computed against the caller’s profile or stored CV. All accept the session cookie or an API key. The skill-match endpoint is deterministic (no LLM); the match-analysis endpoints run the LLM chain and draw on your daily allowance. All take the same facet filter params as search where they narrow a market or candidate set." })
@tagMetadata("Jobs", #{ description: "Public, unauthenticated reads. Jobs are returned in one wire shape (addressed by `public_slug`, never an internal id) shared by the list, detail, company, and search responses. Closed postings are excluded from lists and search and served only by the detail endpoint." })
namespace Freehire;
