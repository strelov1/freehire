## 1. The `present_jobs` tool

- [x] 1.1 Write failing tests in `internal/handler/assistant_tools_test.go` for the tool's argument decoding: a missing `slug`, a missing `note`, an empty `jobs` array and an eleventh entry are each rejected with a message naming what is wrong.
- [x] 1.2 Write failing tests for slug resolution: all slugs resolve → every one in `presented` and `dropped` empty; a mix → the real ones in `presented`, the unknown ones in `dropped` with slug and reason; none resolve → an error naming the unresolved slugs.
- [x] 1.3 Write a failing test asserting the result carries no vacancy payload — only slugs and reasons — so the receipt cannot silently grow into a second copy of the search result.
- [x] 1.4 Implement `presentJobsTool()` in `internal/handler/assistant_present_tool.go`: the JSON schema (`heading?`, `jobs[1..10]` of `{slug, note, why_fits?[≤4], concerns?[≤3]}`), argument decoding via `assistant.DecodeArgs`, and batched resolution through `ResolveSlugsToJobIDs`.
- [x] 1.5 Write the schema's field descriptions as the model reads them: `note` explicitly forbids restating title, company, location or seniority; `why_fits` and `concerns` instruct omission over invention.
- [x] 1.6 Register the tool in `assistantDiscoveryTools()` so both presets offer it, and extend `assistant_preset_test.go` to assert it is present for `chat` and `tailor`.

## 2. The prompt obligation

- [x] 2.1 Replace the canonical-link rule in `internal/assistant/prompt.go` with the presentation rule: a vacancy is shown only through `present_jobs`; a job link in prose is forbidden; `public_slug` is copied from a tool result, never constructed; one call per group; the deck comes first with no preamble before it.
- [x] 2.2 Extend the prompt test to assert the new rule is present and the old "one canonical URL per line" instruction is gone.

## 3. Wire and reducer

- [x] 3.1 Write failing tests in `web/src/lib/assistant/chat.test.ts` for a helper that partitions a message's tool calls into presenting calls and the rest, and that only surfaces a presenting call once its result has arrived and is not an error.
- [x] 3.2 Write a failing test for the join: `result.presented` selects the entries, `input.jobs` supplies each one's note, and an entry named in `dropped` never reaches the deck.
- [x] 3.3 Implement the partition and join as a pure module (`web/src/lib/assistant/deck.ts`) with its typed `PresentedDeck` shape, keeping the parsing out of the Svelte component the way `chat.ts` and `unfurl.ts` already do.

## 4. Deck rendering

- [x] 4.1 Create `web/src/lib/assistant/JobDeckCard.svelte`: `JobRow` in compact mode with a `footer` snippet carrying the note, the `why_fits` chips and the muted `concerns`; falls back to a plain job link when hydration fails.
- [x] 4.2 Create `web/src/lib/assistant/JobDeck.svelte`: the optional heading plus the cards as one spaced group, hydrating each through the existing `jobCache`.
- [x] 4.3 Render decks in `AssistantChat.svelte` and withhold presenting calls from `ToolGroupList`, so no progress chip sits above the deck it produced.
- [x] 4.4 Show a placeholder for a presenting call still in flight, shaped like the deck so the layout does not jump.

## 5. Remove the unfurl path

- [x] 5.1 Delete `web/src/lib/assistant/unfurl.ts`, `unfurl.test.ts` and `JobCardUnfurl.svelte`, and drop their imports and the settled/streaming branch in `AssistantChat.svelte` that existed only to defer unfurling.
- [x] 5.2 Confirm `jobCache.ts` and `JobRow.svelte` are untouched and still used by the deck.

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 6.2 `pnpm --dir web test` and `pnpm --dir web lint && pnpm --dir web build` green.
- [x] 6.3 Drive the assistant end to end against a local server: a search followed by a recommendation renders one deck with the rationale inside each card, and no prose between cards.
- [x] 6.4 Visually verify the deck in light and dark themes at mobile and desktop widths.
