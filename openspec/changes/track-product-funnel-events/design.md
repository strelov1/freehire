## Context

`web/src/lib/analytics.ts` already provides everything needed: a guarded
`track(event, props)` that no-ops until PostHog initializes, plus identity
binding by user id. The five existing events sit inline in `JobsView.svelte` and
`JobView.svelte`, so the established pattern is "call `track()` at the UI action
site", not "wrap the API client".

Three things about the current code shape the decisions below.

**OAuth sign-up is invisible to the browser.** Password registration goes through
`api.register()` (`lib/api.ts:584`), which the client obviously observes. OAuth
is a full-page redirect through `/api/v1/auth/oauth/:provider/start`, so the app
comes back with a session and no way to tell a first-ever sign-in from the
hundredth. The server knows; the client does not.

**A failure reason arrives as prose.** The CV upload rejection the rageclicks
land on is `errResumeNoText` — a full sentence rendered to the user. Sending it
as an event property would make the metric hostage to copy edits.

**`job_track` already covers one path into `applications`, and only one.** It
fires in `confirmApplied()` after `markJobApplied`. Production shows 79 of those
events against 240 rows in `applications`, so most applications are created
elsewhere — mail linking in the inbox, the tracking board, and possibly the
extension.

## Goals / Non-Goals

**Goals:**

- Make the account-side funnel measurable: sign-up, CV upload (including
  failure), application creation, the two credit-charged features, and assistant
  usage.
- Keep every new event PII-free and safe under the existing consent gate.
- Keep failure reasons stable across copy changes.

**Non-Goals:**

- Any backend change, new endpoint, or schema change.
- Charging for anything, or recording assistant token spend — that is
  `meter-the-assistant-turn`, which writes to Postgres for billing. This change
  records that a turn happened, for product analytics.
- Identifying the specific OAuth provider (see Decisions).
- Rewriting or retiring the five existing events.

## Decisions

**Events fire at the UI action site, not inside the API client.** Follows the
existing convention and keeps `lib/api.ts` a pure transport. The alternative —
instrumenting the client — would catch every caller automatically but would tie
analytics to request plumbing and fire on retries and prefetches.

**Sign-up is detected in exactly one place: identity binding.** When a user is
identified and `User.created_at` falls inside a short window before now, the
account is new. `User.created_at` is already on the wire (`lib/types.ts:199`), so
no backend work is needed. Because a reload inside that window would fire the
event twice, a `localStorage` marker keyed by user id makes it once-per-account.

Tracking additionally at the `api.register()` call site was the first plan and is
wrong: a password registration produces a fresh account with `has_password: true`,
so it would satisfy the identify path as well and count twice. One detector,
covering both registration routes, is both simpler and correct.

The method property is therefore `'password' | 'oauth'`, derived from
`has_password` — not the provider name. Recording which provider was used would
require the server to say so on the callback, which is backend work this change
excludes. Coarse method is enough for an activation funnel; provider breakdown can
be added later from the server side.

Alternative considered and rejected: firing `signup` on every first identify of a
browser. That counts a returning user on a new device as a sign-up and inflates
the top of the funnel permanently.

**Failure reasons are a closed set produced by a pure function.** A mapper turns
the server's message into one of a few bounded codes, with an explicit catch-all
so an unrecognised failure still counts rather than vanishing. The function is
plain TypeScript with no framework dependency, which is what the project's vitest
setup can actually test — Svelte components have no test runner here.

**`application_created` is dropped: the browser cannot see the paths that would
justify it.** The plan was an event covering every route into `applications`, to
close the 2,621-clicks against 240-rows gap. Tracing it settled the question
differently. `markJobApplied` in `JobView.svelte` is the *only* client path;
the other writers are server-side — `internal/handler/inbox_linking.go` and
`internal/handler/gmail.go`, which create an application when an email is linked
or Gmail syncs. A front-end event would therefore have duplicated the existing
`job_track` exactly and still missed every row it was meant to explain.

So the gap has an answer, and it is not an instrumentation gap in the browser:
applications are created outside the browser, by mail. Measuring that belongs to
server-side analytics and is deliberately not in this change.

**Everything else is a single `track()` at the action.** `match_run` and
`tailor_run` fire when the user starts the analysis or the tailoring session, not
when it completes — a run that fails or times out still consumed intent and, in
the tailor case, credits. `assistant_message` fires on submit, carrying no text.

## Risks / Trade-offs

**Inferred OAuth sign-up can miss or double-count.** → The `localStorage` marker
bounds double-counting to one per browser per account; the window is generous
enough to survive a slow first load. A user who clears storage and returns inside
the window could re-fire once — acceptable for a funnel, and the exactness lives
on the password path.

**Two events on one path (`job_track` + `application_created`).** → Documented
above and visible in the code; the alternative loses a running production series.

**Component call sites are not unit-tested.** → The project has no Svelte test
runner, so the testable surface is the pure reason-mapper and the sign-up freshness
predicate. The rest is verified by observing the events arrive in PostHog after
deploy, the same way the existing five were.

**Event volume.** → Six new event types against a 1M/month free tier currently
consuming roughly 70k events per 17 days. No practical risk.

## Migration Plan

Frontend-only, no migration. The events are additive; PostHog needs no schema.
Rollback is reverting the commit. The three funnels are configured in the PostHog
UI afterwards and depend only on event names, so they can be built once data
starts arriving.

## Open Questions

Which client paths besides `confirmApplied()` create an application — the inbox
link action and the tracking board are the known candidates, and the exact set
gets pinned down in the task that implements `application_created`.
