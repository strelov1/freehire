## 1. Web lint

- [x] 1.1 Add the same `eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown` comment used in `AssistantChat.svelte` above the `{@html renderMarkdown(...)}` in `MatchAnalysisFull.svelte`
- [x] 1.2 Run `pnpm run lint` in `web/` and confirm zero errors (warnings OK)

## 2. Go format

- [x] 2.1 Run `gofmt -w` on `internal/experience/store_test.go`, `internal/handler/assistant_profile_tool.go`, and `internal/handler/me_experience_test.go`
- [x] 2.2 Confirm `gofmt -l` on those paths prints nothing

## 3. Token ratchet

- [x] 3.1 Rewrite the UUID regex in `web/src/lib/assistant/presets.ts` so the source no longer contains hyphen + `[…]` character-class (false positive for `ARBITRARY`), without changing match semantics
- [x] 3.2 Update `design-system/scripts/web-token-baseline.json` for `web/src/routes/my/+layout.svelte` via the script’s `--update` path after confirming the new count is intentional
- [x] 3.3 Run `pnpm check:tokens` in `design-system/` and confirm exit 0

## 4. Ship locally (no push)

- [x] 4.1 When the user asks, commit the hygiene fixes on `feat/profile-contacts-experience-coverage` (do not push unless they explicitly request it)
