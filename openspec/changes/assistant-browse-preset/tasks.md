## 1. The session API accepts a Bearer credential

- [x] 1.1 In `assistant_integration_test.go` (build tag `integration`, needs Docker), add cases: a valid session JWT sent as `Authorization: Bearer` with no cookie is served; a request with neither cookie nor Bearer is refused; a Bearer credential belonging to a user outside the rollout is refused. Expect them to fail.
- [x] 1.2 Move the five `/assistant/*` routes in `internal/handler/assistant.go` from `mw.cookie` to `mw.key`, and rewrite the `register` comment — the browser that drives it is now also an extension.

## 2. The `browse` preset

- [x] 2.1 Add a failing test that `SystemPrompt(PresetBrowse)` returns a prompt distinct from the chat prompt and instructing the agent to read the page before guessing.
- [x] 2.2 Add `PresetBrowse` beside `PresetChat`/`PresetTailor` in `internal/assistant/store.go` and its prompt in `internal/assistant/prompt.go`, keeping the unknown-preset fallback to the chat prompt intact.
- [x] 2.3 Extend `assistant_preset_test.go`: a `browse` session's registry contains `read_current_page`; a `chat` session's does not; a `tailor` session's does not. (Runs after group 3 — the registry cannot append a tool that does not exist yet.)
- [x] 2.4 Branch on `PresetBrowse` in `assistantHandlers.registry` to append the page tool. (Runs after group 3.)

## 3. The `read_current_page` tool

- [x] 3.1 Add a failing test, over a fake extension end of the hub in the style of `browsertools_integration_test.go`, that the tool issues a `read_page` call and returns the snapshot's url, title, headline and text as structured fields.
- [x] 3.2 Add a failing test that a call with no browser attached returns an error result naming the side panel, and that `Registry.Call` still returns no Go error, so the turn survives.
- [x] 3.3 Write `internal/handler/assistant_page_tools.go`: the tool takes a `Caller` on the caller's channel, issues `read_page`, closes it, and decodes the snapshot. Give `assistantHandlers` its `*browsertools.Hub` and wire it in `newAssistantHandlers` and at the call site in `handler.go`.

## 4. Documentation

- [x] 4.1 Update `internal/assistant/AGENTS.md`: three presets, and the fact that one of them reaches the caller's browser through the relay.
- [x] 4.2 Update `internal/browsertools/AGENTS.md`: the assistant is a second in-process harness beside the autofill run, and record the last-connection-wins consequence of that.

## 5. Creating and listing a browsing session

Found while implementing: `CreateAssistantSession` hard-codes `PresetChat`, so nothing can
create a `browse` session, and `ListAssistantChatSessions` filters `preset = 'chat'`, so one
would be invisible in the rail.

- [x] 5.1 Add a failing integration test that `POST /assistant/sessions` with `{"preset":"browse"}` records a browsing session, that the default (no preset) is still `chat`, and that `tailor` is refused — a tailoring session must come from the bootstrap that knows its CV and vacancy.
- [x] 5.2 Accept an optional `preset` in `CreateAssistantSession`, admitting only `chat` and `browse`.
- [x] 5.3 Add a failing test that a browsing session appears in the caller's session list.
- [x] 5.4 Widen `ListAssistantChatSessions` to `preset IN ('chat','browse')`, update the query comment, and run `make sqlc`.
- [x] 5.5 Found while implementing: migration 0044 pins `preset` with a CHECK, so add `migrations/0047_assistant_sessions_browse_preset.sql` widening it to admit `browse`.

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...`, then `go test -tags=integration ./internal/handler/` for the auth cases.
