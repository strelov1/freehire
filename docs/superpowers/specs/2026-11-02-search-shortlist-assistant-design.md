# Shortlist assistant from the current job search

Date: 2026-11-02.
Status: product scope and written design approved in conversation on 2026-11-02.

## Problem and outcome

An actively searching candidate spends too much time reading job listings. They
already use Freehire's search and want to delegate selection before delegating
applications. Their first useful result is a small shortlist with explanations.

From the standalone jobs list, the candidate opens an assistant beside their
current search. The assistant uses that search, reads the saved profile when
available, and recommends up to five vacancies. The candidate can correct the
selection in ordinary language and request another shortlist. The conversation
can be reopened with its search context intact.

## Scope

- Entry from the standalone `/jobs` list using its current effective query.
- Existing assistant panel, chat, model runner, and recommendation cards.
- A durable search snapshot bound to a conversation.
- Session-local selection preferences learned from the candidate's corrections.
- Explanations grounded in the posting and known candidate information.
- Existing authentication, assistant allowance, cancellation, and retry behavior.

Automatic applications, application-chat handoff, scheduled searches, alerts,
cross-session preference learning, and autonomous submission permission are
separate changes. This task sends nothing to an employer and does not modify the
candidate's global profile.

## Existing implementation

- `web/src/lib/components/JobsView.svelte` owns the live filter store and builds
  the effective search parameters. Its `scope` can also bind embedded company
  lists, which are outside this first entry point.
- `web/src/lib/components/AssistantPanel.svelte` hosts a conversation beside a
  page and keeps it mounted after closing. The jobs layout must make room for
  the dock; the current account-layout offset alone does not establish that.
- `web/src/lib/assistant/AssistantChat.svelte` already supports an existing
  session, an opening message, retry, cancellation, and transcript replay.
- `internal/api/handler/assistant_tools.go` provides discovery tools, but
  `search_jobs` currently accepts only a keyword and string facets. It does not
  reproduce every scalar or caller-specific filter of the jobs list.
- `internal/api/handler/assistant_present_tool.go` and
  `web/src/lib/assistant/JobDeckCard.svelte` already carry a selection note,
  concrete overlaps, and concerns beside catalogue-backed job facts.
- `internal/api/handler/search.go` implements the public query's additional
  behavior, including account-specific exclusions and degraded-filter reporting.
- `internal/api/handler/assistant_profile_tool.go` reads saved candidate context.
  Its current no-profile instruction redirects to the profile page, which must
  not prevent a search-bound conversation from producing a useful shortlist.
- `internal/platform/db/queries/assistant.sql` persists sessions and transcripts.
  The runner replays a bounded history, so the transcript alone is not durable
  storage for the search snapshot or accumulated selection preferences.

## Candidate experience

1. The jobs list offers a localized "Help me shortlist" action. No assistant
   session or model turn starts merely because the page was visited.
2. A signed-out candidate uses the existing sign-in flow with the effective
   search preserved. The assistant retains the existing signed-in requirement.
3. On activation, capture the current live filters, rather than a stale route URL
   or a debounced snapshot that has not caught up with the controls.
4. Open the existing panel and start one new search-bound conversation. Display
   the search being used so the candidate can check its meaning.
5. Read the saved profile if one exists. Produce a shortlist without requiring
   a profile, CV upload, or onboarding form. Ask one consequential clarification
   only when the available information cannot support a useful selection.
6. Show up to five suitable vacancies. Each explanation states a concrete reason
   to consider it and any material concern or unknown. Do not invent candidate
   experience or treat an unspecified employer condition as confirmed.
7. Accept corrections in chat. Persist a compact, session-local selection brief
   from what the candidate actually says, then use it for subsequent selections.
8. Closing and reopening the panel resumes the same conversation. Reloading or
   opening its full assistant URL restores the search context and brief.

Changing filters on the jobs page does not silently change an existing
conversation. The panel keeps showing its captured search and offers an explicit
action to start a new conversation from the current filters. Starting another
search does not cancel an active turn implicitly or overwrite the old context.

## Design decisions

### Reuse the chat with an explicit search binding

Keep the existing `chat` preset and add an optional search binding to a session.
The binding selects scoped discovery behavior and a short additional instruction
at turn assembly. A new preset is unnecessary while the interaction and authority
remain those of the existing search assistant.

The ordinary unbound chat, profile, tailoring, interview, and apply sessions retain
their behavior. A search binding is accepted only for a general chat; other preset
bindings are not combined with it.

### Persist context independently of model history

Add a nullable session search-context value through a new migration and sqlc
queries. It contains the validated, canonical initial search and a bounded
selection brief. The initial search is immutable. The brief may be updated only
through an owner-scoped assistant tool and is injected with the search context on
every turn, including retry and reopening.

The creation API accepts the serialized search parameters, not a model-authored
paraphrase. Validate parameter names, values, and length server-side using the
existing query vocabulary. Strip pagination and transport-only parameters; retain
the effective query, filtering semantics, and supported ordering. An unsupported
condition must be reported rather than silently accepted as enforced.

The selection brief records preferences for this conversation. A one-vacancy
exception remains explicitly local; it does not become a general preference.
It is not stored as candidate experience or as an employer screening answer.

### Share search execution with the list

Extract the necessary request-independent search assembly from the handler into
a narrow helper within `internal/api/handler`. Both the list and the search-bound
tool use it. Do not invoke Fiber handlers internally or construct synthetic HTTP
requests. Keep the current public response and degradation behavior unchanged.

The helper receives context, account identity, and parsed search inputs. It must
retain string facets, scalar bounds, exclusions, AND/OR semantics, title-field
restrictions, and caller-specific filters such as hidden viewed jobs, applied
companies, and former employers. Account-specific sets are resolved for the owner
at execution time rather than stored as stale ID lists in the search snapshot.

Search-bound discovery stays within the captured constraints. Feedback changes
selection and prioritization within that scope. Widening the initial constraints
requires the candidate to explicitly start from revised filters. The assistant
may suggest a relaxation, but does not search outside the scope silently.

The scoped presentation path must use candidates from the scoped search and cap
a shortlist at five. Retain ordinary tool-call and result-size bounds; this task
does not introduce an unbounded scan or promise to exhaust the catalogue.

### Reuse the panel and recommendation cards

Add a small jobs-list host around the shared panel and chat. Initialize on the
first explicit activation, hide instead of unmounting on close, and offer the
existing full-conversation link. Keep the session reference for the current page
and restore it on reload with the captured context; validate ownership on every
server read. A missing or foreign session is not silently recreated as an
unrelated conversation.

Keep filter interpretation and shortlisting as distinct actions: the existing
AI filter dialog edits a query, while this panel evaluates vacancies under a
query the candidate already chose. Do not duplicate the interpreter or replace
the existing dialog as part of this task.

Use existing catalogue-backed cards for the shortlist. Feedback is ordinary
chat in this task; no new rating or feedback taxonomy is needed.

## Failures and limits

- No matches: explain that the current constraints returned no suitable options;
  suggest a specific adjustment without applying it silently.
- An ignored or degraded filter: identify it and do not describe the widened
  result as satisfying the candidate's original constraints. For a shortlist,
  stop and request a supported search instead of recommending widened results.
- No saved profile: select using the current search and stated preferences;
  describe the limited basis for personalization without blocking on profile setup.
- Unknown eligibility or employer policy: show the uncertainty in the concern.
- Search/model failure: preserve the conversation and offer the existing retry.
- Expired login or exhausted allowance: use existing account and plan behavior;
  keep the page's filters and avoid duplicate session creation on repeated clicks.
- Closing the panel does not intentionally cancel a running turn. Stop remains
  the explicit cancellation action.

## Verification

Meaningful automated coverage must demonstrate:

- A snapshot containing salary bounds, negative facets, match-all semantics,
  title-only query fields, and owner-specific filters produces the same search
  inputs as the list. Pagination and tracking parameters do not become filters.
- Unsupported and degraded constraints cannot silently produce a purportedly
  compliant shortlist.
- Session creation, lookup, brief updates, retry, and reopening are owner-scoped;
  another account cannot read or modify the context.
- The initial search is preserved across brief updates and history trimming.
- No-profile behavior supports a useful search-bound opening while ordinary
  chat behavior remains unchanged.
- Search-bound presentation cannot introduce postings outside its candidate
  scope or show more than five in a shortlist.
- Opening creates one session, closing/reopening preserves it, reload restores
  it, and changing page filters does not silently rebind it.
- Failed startup and retry do not create duplicate conversations or duplicate
  opening turns. Both desktop and narrow-screen panel paths remain usable.

Use a bounded model evaluation and a browser walkthrough to assess explanation
grounding and whether a correction changes the next selection. Mocked transport
tests alone do not establish recommendation quality.

Run the relevant Go and frontend tests, type checks, and repository-required
checks for changed files. If generated queries change, regenerate with
`make sqlc`; do not edit generated Go directly. Follow the existing migration,
OpenAPI, localization, and architecture rules.

## Delivery boundary

The implementation is complete when a candidate can start from an existing
search, get an explained shortlist, refine it, and resume the same conversation
without losing its search context. Application preparation remains the next task.
