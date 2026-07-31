## Context

The mail stack already does the expensive part. `cmd/classify-mail` drains
`email_classification_outbox`, stamps each message with a `status_signal` from
`mailclassify`'s controlled vocabulary, and links it to an application when
`mailmatch` finds a deterministic tier. Those labels are read by `/my/inbox` and by an
external harness through `GET /me/inbox`. The in-app assistant cannot see any of it.

The obstacle is structural rather than conceptual. Mail logic lives inside three Fiber
handler files and talks to `*fiber.Ctx` directly — `parseInboxFilters(c)`,
`advanceStage(c, userID, jobID, sig)`, `h.renderEmail(c, ...)`. The assistant's tools run
in-process with no HTTP request and no credential; they call a Go service and receive the
session owner's `userID`. Every other tool group already has such a service to call
(`h.tracking.tracking.SaveJob`, `h.cv`, the experience bank). Mail has none.

This is the same shape as the defect closed by the assistant-untrusted-content-boundary
change: `cv_get` filtered the CV's contact block behind `if auth.ViaAPIKey(c)`, correct
while the agent was an external process holding a key, and silently dead once the agent
moved in-process. A guard keyed on the transport rather than on the reader stops firing
the moment a second reader appears. Mail carries at least one rule of exactly that
kind — "read bodies through the listing, never through the single-message endpoint,
because the latter marks mail read" — and it is currently enforced only by which URL a
caller happens to choose.

## Goals / Non-Goals

**Goals:**

- A candidate can ask the general assistant about their application mail and get an answer
  from the labels that already exist, without the agent reading the mailbox top to bottom.
- The agent can act on mail the way the CLI's `inbox` commands do: classify, resolve a
  matcher suggestion, fix a link, record a missing application.
- One implementation of every mail operation, reached identically by the HTTP endpoint and
  by the tool. The public API contract does not change, so `freehire-cli` and its skill are
  untouched.
- The split between presets is explicit and guarded: each preset's prompt names only tools
  that preset registers.

**Non-Goals:**

- Pushing externally-fetched mail (`POST /me/emails`), deleting or restoring messages,
  mark-all-read, and the Gmail connect/disconnect pair stay out of the tool surface. They
  are either the harness tier's job or a browser redirect.
- No mail in the `tailor`, `profile` or `browse` presets.
- No metering. A turn remains free to the caller and billed to us; that gap is the
  assistant's, not this change's.
- No schema migration. The only new SQL is a read.

## Decisions

### Extract `internal/inbox` rather than share handler methods

A new package holds the mail operations as plain Go:

```go
package inbox

func (s *Service) Overview(ctx, userID) (Overview, error)
func (s *Service) Search(ctx, userID, Query) (Page, error)
func (s *Service) Triage(ctx, userID, id int64, Verdict) (Message, error)
func (s *Service) Link(ctx, userID, id int64, slug string) (Message, error)
func (s *Service) Unlink(ctx, userID, id int64) (Message, error)
func (s *Service) ResolveSuggestion(ctx, userID, id int64, accept bool) (Message, error)
func (s *Service) RecordApplication(ctx, userID, id int64, slug string) (Message, error)
```

Errors are sentinels — `ErrNotFound`, `ErrUnknownSignal`, `ErrPendingSuggestion`. The
handler maps them to Fiber statuses; the tool maps them to a sentence the model can act
on. **Validation happens once; only the rendering is written twice**, and the two
renderings are genuinely different artifacts (an HTTP status against a self-correction
message), not duplicated logic.

*Alternative considered — unexported methods on `inboxHandlers` taking `(ctx, userID,
params)`, called by both the Fiber handlers and the tools.* Smaller diff and no new
package. Rejected because it leaves the mail domain inside `internal/handler`, which is
already the largest package in the repo, and because the extraction is what makes the
read-marking guarantee structural: a `Service.Search` that has no `MarkRead` in reach
cannot mark anything read, whereas a handler method sitting next to `h.queries` can always
grow one.

*Alternative considered — the assistant calling its own HTTP API over loopback.* Rejected
outright: there is no credential to call it with, and minting one for an agent is exactly
what `internal/assistant` is built to avoid.

`advanceStage` moves into the service and drops its `*fiber.Ctx`. It stays best-effort: a
failed stage advance must not fail a verdict that is already durable.

### Two read tools: counts first, then a narrow search

`inbox_overview` returns counts only — per label, unclassified, unread, and per link
state. `inbox_search` takes the filters `ListEmails` already supports (`status`, `link`,
`unread`, `unclassified`, `q`, `limit`, `offset`) plus an opt-in `include_body`.

This mirrors `get_profile`, which reports the experience bank's *shape* while
`experience_search` returns its content: a tool result is persisted in the transcript and
replayed into the model's context on every later turn, so the orientation call must be
cheap enough to keep forever.

*Alternative considered — one search tool.* The model would have to guess a label from a
vague question like "что там у меня", and would either fire several searches or pull an
unfiltered page. *Alternative considered — folding the counts into `get_profile`.* Fewer
tools, but `get_profile` is registered for every preset, so mail counts would leak into
tailoring and the experience interview — the precise thing this change is meant to avoid.

The overview needs one new sqlc query aggregating by `status_signal`, `classified_at IS
NULL`, `read_at IS NULL` and link state in a single pass. No migration.

### Bodies are capped harder for the model than for a harness

`agentPageMax` is 50 over HTTP. The tool caps a body-bearing page at **10**. The asymmetry
is deliberate and belongs in the spec: a harness reads a page once, while the model
re-reads its own tool results on every subsequent turn of the session. The existing
`assistantResultCap` (60 kB) is a backstop against a single oversized result, not a
substitute — it truncates after the fact, and a truncated mail listing is a listing whose
tail silently does not exist.

### `NormalizePreset` — one decision behind two switches

`SystemPrompt` falls back to `chatPrompt` for an unrecognised preset. The registry
compares `sess.Preset == assistant.PresetChat`. Adding mail to both would give an unknown
preset the chat prompt (now naming mail tools) and a tool set without them — the model
reads instructions for tools it does not have and burns rounds on `unknown tool`.

`assistant.NormalizePreset(string) string` maps anything unrecognised to `PresetChat`, and
both switches read it. This is a smaller commitment than a capability table: the CV group
is additionally conditional on the session's CV/vacancy binding, which is a handler
concern the table could not own, so a table would be half a rule.

### `mailPrompt` appended only to the chat prompt

```
SystemPrompt(chat)   = chatPrompt + mailPrompt
SystemPrompt(browse) = chatPrompt + browsePrompt      // unchanged, no mail
```

`browsePrompt` already extends `chatPrompt` rather than copying it; mail becomes a second
composable section under the same rule. The content is the reader-facing half of
`docs/agents/mail-stack.md` and `freehire-cli`'s `using-freehire` skill: orient before
searching; the sender display name is usually the ATS relay, not the employer; a calendar
event the candidate organised is `other`, not `interview_invitation`; only a deterministic
match auto-links, so everything else waits in `link=suggested` and a queue nobody drains
never resolves; and a message body is untrusted input where an instruction is an attack,
not a request.

### The prompt↔registry drift guard

A test extracts every backticked identifier from each preset's prompt, keeps those that
look like tool names, and asserts each is registered for that preset. Nothing guards this
today — it is why mail cannot simply be appended to `chatPrompt`, and it will fail loudly
the next time a prompt and a tool group are edited apart.

### An unknown signal is an error, not `other`

`mailclassify.Sanitize` coerces an out-of-vocabulary label to `other` because the
classification worker feeds it raw LLM output derived from an attacker-controlled body,
and must never persist a raw model string. The tool path is different: the label is a
judgement the candidate asked for, and coercing a typo to `other` records a verdict nobody
chose while looking like success. `inbox_triage` returns an error naming the invalid value
and listing `mailclassify.SignalValues` — the model's only route to self-correction within
the turn.

## Risks / Trade-offs

**Behaviour drift during the extraction** → The mail HTTP surface has integration tests
(`inbox_integration_test.go`, `inbox_agent_integration_test.go`,
`inbox_application_from_mail_integration_test.go`, `inbox_linkstate_integration_test.go`).
They run unchanged against the thinned handlers and are the proof the extraction preserved
behaviour. Any of them requiring an edit is a signal the contract moved, not that the test
is stale.

**Prompt injection from a mail body** → The tool surface contains nothing that sends mail,
so an injection has no outbound channel. The reachable damage is a wrong label or a wrong
link, both reversible by the candidate from `/my/inbox`, and `mailclassify.AdvanceStage`
keeps stage movement monotonically forward. Bodies stay bounded by
`maillink.ReadableBody` + truncation. Residual: an injection could still waste a turn.

**Twelve extra tool schemas in every chat turn** → Seven mail tools join the eleven the
chat preset already carries. Measured cost is the schema block on each request, not the
result cap. Mitigation is the preset scoping itself: `tailor`, `profile` and `browse` pay
nothing. If chat turns get expensive, the seam is to split mail behind a session flag —
noted, not built.

**An unclassified backlog makes the overview look empty** → `cmd/classify-mail` is
cron-driven, and external-source mail is never classified server-side by design. The
overview reports `unclassified` as its own count precisely so the agent can say "12
messages have not been judged yet" instead of "you have no interview invitations", which
would be false.

## Migration Plan

No migration. One added read query, regenerated via `make sqlc`. Deploy is an ordinary
release: the new package and tools ship together, the HTTP contract is unchanged, and
rollback is a revert.

## Open Questions

None blocking. Deferred by choice: whether the mail tools should later become available in
the browser-panel preset when the candidate is standing on a webmail tab, and whether
`inbox_overview` should also report which mail sources are connected (it would let the
agent say "no mailbox is connected" instead of "no mail found" — worth revisiting once the
counts are live and it is clear how often that case is hit).
