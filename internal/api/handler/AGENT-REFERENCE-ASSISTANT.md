# internal/api/handler — the assistant surface

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md): sections moved out of it for size.

## Assistant (`assistant.go`, `assistant_*_tools.go`)

`mountMCP` (`mcpapp_mixed.go`) mounts `mcpapp.Handler` at `/mcp` behind
`mcpAuthenticate`, which requires a bearer credential (no anonymous session; see
[mcpapp](../mcpapp/AGENTS.md)). `mcpKeyTools` / `mcpAccountTools` pick the set;
`search_jobs`, `get_job` and `get_company` in it are never served, the catalogue answers
those names (`searchJobs()` and the rest are the in-app assistant's alone).

The OAuth account MCP also exposes `suggest_dictionary_term`. It reuses
`dictionaryHandlers.recordTerm`, including dictionary alias checks and the website's
atomic queue upsert. Request admission belongs to the operations gateway. It only
records a request for review, never
installs a filter. Agents submit a term the user wanted but the dictionary cannot
resolve, without a separate suggestion request; zero search results alone do not
prove a dictionary gap. Like the cookie-only dictionary endpoint, it is unavailable to
full-scope API keys. `handler.go` constructs one dictionary handler for both surfaces.


Routes (all on `mw.key` — the session cookie, the session JWT the extension's
connect flow minted, or a full-scope API key — and nothing else). Authentication
is the whole gate: every signed-in user reaches the assistant. The gate is `key`
rather than `cookie` because the extension's side panel holds conversations too
and cannot send an httpOnly cookie across origins. **Every turn is metered** —
it draws on the caller's plan before the stream opens, and a spent allowance is a
402 rather than an event inside a 200 (`assistant_meter.go`). A tailoring turn is
the exception: it draws on no assistant allowance, because its session carries
its own two bounds:

| Route | Does |
|---|---|
| `POST /assistant/sessions` | start a conversation in the preset asked for — `chat`, `profile`, `browse`, `interview` or `debrief`; a tailoring one is minted by the CV bootstrap, which knows the CV and vacancy to bind |
| `GET /assistant/sessions` | the caller's conversations, most recently active first |
| `GET /assistant/sessions/:id` | one conversation with its stored transcript, for replay |
| `DELETE /assistant/sessions/:id` | remove a conversation and its transcript |
| `POST /assistant/sessions/:id/messages` | run one turn, streamed as named SSE events; the body may carry up to three `attachments` (`{name, text}`) beside `text` |
| `POST /assistant/attachments` | read one PDF (multipart `file`) and return its text for the next message — stores nothing and makes no model call |
| `POST /assistant/sessions/:id/retry` | resume after a failed turn without appending another user message (same SSE stream) |
| `POST /assistant/sessions/:id/cancel` | stop a running turn (owner-scoped — see below) |
| `POST /assistant/sessions/:id/extend` | buy a tailoring session another ceiling's worth of turns, out of the day's tailoring allowance; 409 on any other preset |
| `POST /assistant/sessions/:id/opening` | the assistant speaks first in a rehearsal or debrief, under a server-side brief; an already-answered opening is a 409 |
| `POST /support/sessions` | start a support conversation (**cookie-only**, every signed-in account, no paid gate) |
| `POST /support/sessions/:id/messages` | one support turn, SSE. Free admission, own meter: a flat daily allowance (402), a per-session ceiling (429) and a global daily budget (429); refusals carry `support_email` and never an `upgrade_url`. 404 for any non-support session |
| `POST /support/sessions/:id/cancel` | stop a running support turn |
| `POST /assistant/sessions/:id/voice-token` | mint one voice call's credential |
| `POST /assistant/sessions/:id/voice-turns` | append a completed spoken turn to the transcript |
| `POST /assistant/sessions/:id/autopilot` | run the tailoring pass unattended — same stream, server-owned brief and ceiling (**cookie-only**) |

A session the caller does not own is a 404, never a 403, so ids stay unprobeable.
The autopilot route departs from the `key` gate above: it rewrites a CV without
being asked anything, and the browser is the one place the candidate can watch it
happen and undo it. It refuses anything but a tailoring session bound to a CV
(**409**). There is no pre-run snapshot any more: every edit the agent makes in
the turn is filed under one revision batch, so "undo the run" is undoing that
batch. Undoing it is `POST /me/cvs/:id/revisions/batch/:bid/undo` (also
cookie-only), which reverts the batch's standing edits newest-first and clears
the run's report with it.

Both SSE endpoints — the turn and the fit analysis — write through `sseStream`
(`match_analysis_stream.go`), the one owner of the stream protocol: it serializes the
heartbeat goroutine against the event callback and re-arms the write deadline before every
write. `event` reports whether the frame reached the client, and a marshal failure reports
**true** — an unencodable frame is our bug, not a dead reader.

**The fit stream refuses a cookie request it cannot attribute to the site**
(`cookieOnlyStreamAllowed`, checked at the top of `StreamMatchAnalysis`, after the route's
mounted gates). The session cookie is SameSite=Lax, and a cross-site TOP-LEVEL GET
navigation (anchor, `window.open`, address-bar redirect) carries it by spec — onto a route
that reserves an AI credit and runs the model. A cookie-authenticated request stamped
`Sec-Fetch-Site: cross-site` — or carrying no Fetch Metadata at all, which proves nothing
and must not spend a credit — is a 403. `same-site` is admitted beside `same-origin`, an
explicit assumption that every origin under the site's control is trustworthy. A scripted
caller has a first-class path beside this gate: a Bearer API key, which cannot ride a
navigation and is never gated by it. If a second route ever mutates state on GET behind
the session cookie, it takes this gate; do not re-derive the policy per route.

**Every goroutine a stream starts recovers its own panic** (`recoverStream`), because the
server's recover middleware cannot reach any of them: `SetBodyStreamWriter` runs its
closure in a bare `go sw(bw)` after the handler chain has already returned nil, so a panic
there is not a 500 — it is `cmd/server`, with every other request in flight. The recover
reports to the request's cloned hub and writes the one terminal frame that stream still
owes (`result`/`stop_reason: error` for a turn, `stream_error` for the other two); the
heartbeat passes nil, since its body writer is still there to close the stream. Register it
FIRST inside the closure so it unwinds LAST, after the defers that release the allowance,
the turn slot or the analysis claim — those are what make the terminal frame true. For the
same reason the heartbeat's `stop` is always deferred: a panic that skipped it would leave
the ticker writing into a `bufio.Writer` fasthttp has already recycled onto another
connection.

A failed write does NOT stop an assistant turn. It used to: the failure cancelled the loop's
context, which meant a phone freezing a backgrounded tab threw away live work — an unattended
tailoring run once lost its report after twenty-five committed CV edits. A write that fails now
means only that this reader is not listening; the turn runs to its own end under the step cap
and the model timeout, and its transcript is stored either way. The fit analysis always ignored
the same signal, for the neighbouring reason: its run was paid for out of the day's allowance.

Stopping a turn is therefore a request of its own — `POST /assistant/sessions/:id/cancel`,
owner-scoped — because a dropped connection cannot be told apart from a deliberate Stop.
`turnRegistry` (`assistant_turns.go`) holds each running turn's cancellation, which is the only
handle on a turn no request owns any more, and keeps a session to ONE turn at a time: a second
message waits for the first and is told so with a `queued` event, a third is refused with 409.
The registry is per-process, so a turn that outlives a blue/green flip cannot be cancelled and
ends at its step cap.

`assistant_tools.go` / `assistant_tracking_tools.go` / `assistant_cv_tools.go` /
`assistant_profile_tool.go` / `assistant_present_tool.go` / `assistant_page_tools.go`
build the agent's tools from the same services these handlers use, and
`assistantRegistry` picks the set for a session's preset. The loop itself lives in
[internal/ai/assistant](../../ai/assistant/AGENTS.md).

`read_current_page` (`assistant_page_tools.go`) is the only tool that leaves this
process for something we do not control: it drives the caller's browser through the
relay in [internal/ai/browsertools](../../ai/browsertools/AGENTS.md). It is registered for the
`browse` preset alone, and it carries its own deadline, because every other tool is
bounded by a database or a search call and this one waits on a tab.

`present_jobs` is the odd one out: it is the only tool whose purpose is presentation
rather than retrieval or state change. It writes nothing and returns a receipt of
slugs, not vacancies — the client renders the deck and fetches each card's data by
slug, so a recommendation costs the model's context once, not twice.

`get_profile` is built on `profileHandlers` rather than the services under it, so the
tool and `GET /me/profile` share one assembly and cannot drift. It returns the CV as
`resumeextract.Professional` — **contacts omitted**, and not merely as good manners: a
tool result is persisted in the transcript and replayed into the model's context on
every later turn, so a name that lands there stays for the conversation's life. A
caller with no profile gets a result naming `/my/profile`, never an error and never an
empty profile the model would read as "no preferences".
