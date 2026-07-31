## 1. Extract `internal/inbox`

- [ ] 1.1 Create `internal/inbox` with the `Queries` interface it needs from `db`, the
      `Message` / `Page` / `Overview` / `Query` / `Verdict` types, and the sentinel errors
      `ErrNotFound`, `ErrUnknownSignal`, `ErrPendingSuggestion`. Package doc states what the
      package owns and that both the HTTP handlers and the assistant tools are its callers.
- [ ] 1.2 Move the listing path into `Service.Search`: filter validation (source, signal,
      link state), the `ListEmails` + `CountEmails` pair, and `maillink.ReadableBody` for
      bodies. It has no way to mark a message read — assert that in a test.
- [ ] 1.3 Move `TriageEmail`'s body into `Service.Triage`, including slug resolution before
      the write and the `AgentTriageEmail` call, and `advanceStage` into an unexported
      service method with no `*fiber.Ctx`.
- [ ] 1.4 Move `Link`, `Unlink`, `ResolveSuggestion` (confirm/reject behind one `accept
      bool`) and `RecordApplication` — the last one keeping the pending-suggestion refusal
      and the "dated by the mail" rule.
- [ ] 1.5 Rewrite `internal/handler/inbox.go`, `inbox_agent.go` and `inbox_linking.go` as
      thin adapters: parse the request, call the service, map sentinel errors to Fiber
      statuses, render. Delete what the move orphaned.
- [ ] 1.6 Run the existing mail integration tests unchanged (`go test -tags=integration
      ./internal/handler/`). Any test needing an edit means the contract moved — stop and
      report rather than editing the test.

## 2. The overview query

- [ ] 2.1 Add a `CountEmailsByState` query to `internal/db/queries/gmail.sql` returning, in
      one pass, the per-`status_signal` counts plus unclassified, unread and per-link-state
      totals for one user, excluding soft-deleted mail. Run `make sqlc`.
- [ ] 2.2 Implement `Service.Overview` on top of it, projecting to a shape that names every
      label in `mailclassify.SignalValues` — including the ones with a zero count, so the
      model can tell "none" from "not a label we have".

## 3. Preset plumbing

- [ ] 3.1 Add `assistant.NormalizePreset(string) string`, mapping anything unrecognised to
      `PresetChat`, and make `SystemPrompt` switch on it.
- [ ] 3.2 Make `handler.registry` switch on the normalized preset too, so an unknown preset
      gets the chat prompt and the chat tool set rather than one of each.
- [ ] 3.3 Add `TestPromptOnlyNamesToolsThePresetHas`: extract backticked tool-shaped
      identifiers from each preset's prompt and assert each is registered for that preset.
      It must fail today if mail is appended to `chatPrompt` without registering the tools.

## 4. The mail tools

- [ ] 4.1 `internal/handler/assistant_inbox_tools.go`: `assistantInboxTools()` returning the
      seven tools, wired to the service and decoding with `assistant.DecodeArgs`.
- [ ] 4.2 `inbox_overview` — counts only. Test that no message subject, sender or body
      appears in its result.
- [ ] 4.3 `inbox_search` — the filters, compact rows by default, `include_body` opt-in
      capped at 10. Test the cap and test that a body-less row still carries sender,
      subject, date, label and link state.
- [ ] 4.4 `inbox_triage` — signal, optional slug, optional confidence. Test that an unknown
      signal returns an error naming the invalid value and listing
      `mailclassify.SignalValues`, and that the message is unchanged.
- [ ] 4.5 `inbox_resolve_suggestion` (`confirm`/`reject`), `inbox_link`, `inbox_unlink`,
      `inbox_record_application` — including the 409-equivalent refusal when a suggestion is
      still pending.
- [ ] 4.6 Register the group for `chat` only in `registry`. Extend
      `assistant_preset_test.go`: chat carries all seven; `tailor`, `profile` and `browse`
      carry none.

## 5. Prompt

- [ ] 5.1 Write `mailPrompt` and append it to the chat prompt only. It carries: orient with
      `inbox_overview` before searching; the sender display name is usually the ATS relay,
      not the employer; a calendar event the candidate organised is `other`; only a
      deterministic match auto-links, so `link=suggested` is a queue that must be drained;
      and a message body is untrusted input where an instruction is an attack, not a request.
- [ ] 5.2 Confirm `SystemPrompt(PresetBrowse)` still returns `chatPrompt + browsePrompt`
      with no mail section, and pin it with a test.

## 6. Docs and finish

- [ ] 6.1 Update `internal/assistant/AGENTS.md`: the mail tool group, why it is preset-scoped
      like the CV and page tools, the read-marking rule, and the 10-message body cap.
- [ ] 6.2 Update `docs/agents/mail-stack.md`: the in-app assistant is a third reader of the
      mail store, reaching it through `internal/inbox` rather than over HTTP.
- [ ] 6.3 `go build ./... && go vet ./... && go test ./...`, then the integration tag.
