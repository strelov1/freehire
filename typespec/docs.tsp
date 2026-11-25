import "@typespec/openapi";
import "./docs-overview.tsp";

using TypeSpec.OpenAPI;

// The reader-facing half of the contract: the overview every API reference opens with and
// the description of each tag. web/scripts/gen-api-docs.mjs renders the docs from
// openapi/api.openapi.json, so what is written here is what /docs/api and docs/API.md say.

@doc(ApiOverview)

// Listed last-first: decorators apply bottom-up, and the reference shows the tags in
// the order they are applied.
@tagMetadata("Change webhooks", #{ description: "Push delivery of the change feed to your own endpoint. Session or API key; needs an `api` or `api_data` plan. See Change webhooks above for the payload and how to verify a signature." })
@tagMetadata("Catalogue export", #{ description: "Full-catalogue files for the Data plan (`api_data`). A snapshot is one gzip file, `jobs.jsonl.gz`: UTF-8 JSON Lines, one job per line, each in the same wire shape as `GET /jobs/{slug}` (full `description` included), covering the open, canonical, public postings the search index lists. The file is streamed, so it is not one JSON document; read it line by line. Every snapshot has a `schema_version`, a row count, a byte size and a SHA-256 to verify the download. Snapshots are named by UTC day and the last few are kept. How often a new one appears is set by the operator timer, not by this API: read `date` and `generated_at` rather than assuming a schedule. To stay current between snapshots, re-fetch single jobs with `GET /jobs/{slug}`. The licence is the API plan's: show the data in your own product, do not resell it as a dataset or feed, and stop showing it when the plan ends — see /terms#data-licence." })
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
@tagMetadata("Jobs", #{ description: "Public, unauthenticated reads. Jobs are returned in one wire shape (addressed by `public_slug`, never an internal id) shared by the list, detail, company, and search responses. Closed postings are excluded from open lists and search. The detail endpoint keeps serving them. Interview prep can serve a closed posting to the account tracking it at the interview stage." })
namespace Freehire;
