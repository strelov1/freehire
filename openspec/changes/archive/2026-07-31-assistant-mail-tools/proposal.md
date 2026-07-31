## Why

A candidate's application mail is already classified — `cmd/classify-mail` stamps every
inbound message with a `status_signal` and, where the match is deterministic, links it to
the application it belongs to. That work is reachable from `/my/inbox` and from a user's own
harness through the `freehire` CLI, but not from the in-app assistant. So the one question
the labels exist to answer — *"what's happening with my interview invitations?"* — cannot be
asked in the place the candidate is already having the conversation, and answering it by
hand means opening the inbox and reading down the list.

## What Changes

- Extract `internal/inbox`: a service layer over the mail queries (`Overview`, `Search`,
  `Triage`, `Link`, `Unlink`, `ResolveSuggestion`, `RecordApplication`) with plain
  `(ctx, userID, params)` signatures and sentinel errors. The three inbox handler files
  become thin Fiber adapters over it. **The public HTTP contract does not change** — the
  CLI and its skill keep working untouched.
- Add a counts query behind `Overview`: how many messages carry each label, how many are
  unclassified, how many unread, and how many sit in each link state.
- Give the assistant seven mail tools — `inbox_overview`, `inbox_search`, `inbox_triage`,
  `inbox_resolve_suggestion`, `inbox_link`, `inbox_unlink`, `inbox_record_application` —
  registered **only** for the `chat` preset.
- Add a `mailPrompt` section appended only to the chat prompt. `browse` keeps
  `chatPrompt + browsePrompt` and gains no mail instruction, because it gains no mail tool.
- Add `assistant.NormalizePreset`, read by both `SystemPrompt` and the handler's registry,
  so an unrecognised preset cannot resolve to one prompt and a different tool set.
- Add a drift guard: for every preset, each tool named in its prompt must be registered for
  that preset, and vice versa.

Out of scope: pushing externally-fetched mail, deleting or restoring messages, mark-all-read,
Gmail connect/disconnect, opening a single message by id, mail in the `tailor`/`profile`/
`browse` presets, and metering a turn.

## Capabilities

### New Capabilities
- `assistant-mail-triage`: what the in-app assistant may do with the candidate's mail — the
  orientation-then-search read path, the triage and linking actions, the bounds that keep a
  tool result small, and the reason the assistant is denied the read-marking single-message
  endpoint.

### Modified Capabilities
- `assistant-agent-runtime`: the tool surface gains the mail tools; the preset requirement
  gains a third scoped group (mail, beside CV and page) and the rule that a preset's prompt
  and its tool set are chosen by one shared decision rather than two parallel switches.

## Impact

- **New code:** `internal/inbox` (service, moved from `internal/handler/inbox*.go`),
  `internal/handler/assistant_inbox_tools.go`, `assistant.NormalizePreset`, `mailPrompt`.
- **Moved code:** `GetInbox`/`GetEmail`/`TriageEmail`/`advanceStage`/the linking handlers keep
  their routes and responses; their bodies move into the service. `advanceStage` stops taking
  `*fiber.Ctx`.
- **SQL:** one new sqlc query for the overview counts. No migration — no schema change.
- **API:** no new or changed endpoints. The assistant reaches mail in-process, as every other
  tool does.
- **Specs preserved without a delta:** `agent-inbox-surface` and `application-from-mail` keep
  every requirement they have; the extraction is behaviour-preserving and their existing
  integration tests are what proves it.
- **Dependencies:** reuses `internal/mailclassify` (signal vocabulary, `AdvanceStage`),
  `internal/maillink` (`ReadableBody`), and `db.Queries`. Nothing new.
