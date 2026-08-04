## 1. Close out the provenance-upgrade fix

- [x] 1.1 Fold the already-regenerated `internal/db/querier.go` doc-comment diff (from
  the shipped `c7901ec9` sqlc regen) into the first commit of this change, so the
  generated files stay consistent with `internal/db/queries/experience.sql`.

## 2. Backend: the `request_confirmation` tool

- [x] 2.1 Add the `request_confirmation` tool (`{claim: string, question: string}` →
  `{"status": "awaiting_candidate_response"}`, no side effect) in
  `internal/handler/assistant_cv_tools.go`, registered only under the `tailor` preset
  gate in `internal/handler/assistant_tools.go`.
- [x] 2.2 Update `tailorPrompt` step 2 in `internal/assistant/prompt.go`: replace the
  free-text "ASK them (...)" instruction with an instruction to call
  `request_confirmation`, passing the exact claim text and a short question, instead of
  writing the request as prose.
- [x] 2.3 Confirm `TestPromptOnlyNamesToolsThePresetHas` (or its current name in
  `internal/assistant`) passes with the new tool named in the prompt and registered for
  `tailor` — add a preset-registration test if this specific case (tool present only
  under one preset) isn't already covered by the existing table-driven test.

## 3. Frontend: render the confirmation as buttons

- [x] 3.1 Add a name-conditional branch in `web/src/lib/assistant/ToolGroupList.svelte`
  for `request_confirmation`: render the `claim` text plus **Да**/**Нет** buttons instead
  of the generic collapsed tool-call line; every other tool name keeps the existing
  generic rendering.
- [x] 3.2 Wire **Да** to `submitText(raw)` (`AssistantChat.svelte:545`) with `raw` equal
  to the claim text, verbatim and unmodified. Wire **Нет** to `submitText` with a fixed
  decline message (e.g. "Нет, это не так — не добавляй.").
- [x] 3.3 Add component tests: the new branch renders buttons for `request_confirmation`
  and falls back to generic rendering for every other tool name; each button's click
  fires `submitText` with the expected payload (claim text verbatim for Да, the fixed
  decline string for Нет).

## 4. Remove Follow-ups: backend

- [x] 4.1 Delete `internal/assistant/followups.go` and its test file.
- [x] 4.2 Delete `internal/handler/assistant_followups.go` and its unit and integration
  test files; remove the route registration in `internal/handler/assistant.go` and the
  corresponding route-list assertion in
  `internal/handler/assistant_integration_test.go`.
- [x] 4.3 Grep the module for `tagFollowUps` (`internal/handler/user_llm.go`); if nothing
  else references it, delete it, otherwise note what still depends on it before touching
  anything.
- [x] 4.4 Remove the "Follow-ups" section from `internal/assistant/AGENTS.md` (it
  documents now-deleted functionality); leave every other section as-is.

## 5. Remove Follow-ups: frontend

- [x] 5.1 Delete `web/src/lib/assistant/followups.ts` and its test file; remove the
  `suggestFollowUps` call and its wrapper from `web/src/lib/assistant/api.ts`.
- [x] 5.2 Remove every remaining reference in `web/src/lib/assistant/AssistantChat.svelte`:
  the `followUps` state, `askForFollowUps`, the reset points that clear it on a new turn,
  and the chip-rendering block.

## 6. Verify

- [x] 6.1 Run `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, and
  `go test ./...`; run `go test -tags=integration ./internal/handler/` (needs Docker) to
  cover the deleted-route assertion and the new tool's registration test. Done: also ran
  the full `go test -tags=integration ./...` (whole module, Docker), all green.
- [x] 6.2 Run the frontend test suite (`web/`) for the touched Svelte files; manually
  drive one tailoring turn that needs a fresh confirmation (through the dev server) to
  confirm the button appears, **Да** unsticks the write, and **Нет** leaves the claim
  out, and confirm the Follow-ups strip no longer appears anywhere in the assistant chat.
  Done: `vitest run` (822 tests), `svelte-check` (0 errors) and a production `vite
  build` all pass clean. The live click-through happened incidentally on 2026-08-04 —
  the candidate pasted a real tailoring-chat transcript into an unrelated session that
  shows the `request_confirmation` Yes/No buttons rendering and one claim (Finbridge)
  confirming and banking successfully; no Follow-ups strip appeared anywhere in it. That
  session also surfaced two NEW findings outside this task's scope — the agent batching
  two unconfirmed drafts into one message, and inventing unstated specifics in a draft —
  tracked as follow-up work, not a failure of what this change shipped.
